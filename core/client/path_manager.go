package client

import (
	"context"
	"crypto/rand"
	"fmt"
	"log/slog"
	"math"
	"math/big"
	"time"

	"example.com/scion-time/net/scion"
	"example.com/scion-time/net/udp"
	"github.com/scionproto/scion/pkg/addr"
	"github.com/scionproto/scion/pkg/snet"
)

type PathManager struct {
	RemoteAddr               udp.UDPAddr
	LocalAddr                udp.UDPAddr
	S                        []snet.Path
	S_Active                 []snet.Path
	StaticSelectionInterval  time.Duration
	DynamicSelectionInterval time.Duration
	Pather                   *scion.Pather
	Cap                      int              // total active paths you want to keep
	K                        int              // total candidate paths to consider
	Probers                  [20]*SCIONClient // handle path assessment (symmetry, jitter), LENGTH TO BE DEFINED SOMEWHERE ELSE
}

/*
func (pM PathManager) GetPaths(ctx context.Context, log *slog.Logger, cap, k int, remoteAddr udp.UDPAddr) []snet.Path {
	/*if pM.LastSelection.IsZero() || time.Since(pM.LastSelection) >= pM.SelectionInterval {
		pM.LastSelection = time.Now()
		file, _ := os.Create("output.txt") // overwrites if file exists
		defer file.Close()

		log.LogAttrs(ctx, slog.LevelDebug, "STATIC SELECTION",
			slog.Any("------", "------"))
		// s := "71-20965" // Geant
		s := "67-401500" // north america
		address, _ := addr.ParseIA(s)
		// log.Debug("Address formating", slog.Any("error", err))
		ps_temp, _ := pM.Pather.GetPathsToDest(ctx, scion.DC, address)
		for i, path := range ps_temp {
			fmt.Fprintf(file, "Path %d: %d\n", i+1, len(path.Metadata().Interfaces))
		}

		log.Debug("printing paths", slog.Any("#paths", len(ps_temp)))
		ps_temp_selected := chooseNewPaths(ps_temp, k) //[]snet.Path
		for i, path := range ps_temp_selected {
			fmt.Fprintf(file, "Path %d: %d\n", i+1, len(path.Metadata().Interfaces))
		}
		for i, path := range ps_temp_selected {
			fmt.Fprintf(file, "Path %d: %s\n", i+1, path.Metadata().Interfaces)
		}
		log.Debug("printing selected paths", slog.Any("#paths", len(ps_temp_selected)))

		// find the best cap performing
		//initPaths := client.PickRandom(ps_temp_selected, cap)!!!!!!!!!!!!!!!!!!!!!!!!!!!!!
		for i, path := range initPaths {
			fmt.Fprintf(file, "Path %d: %s\n", i+1, path.Metadata().Interfaces)
		}
	}
	if pM.StaticLastSelection.IsZero() || time.Since(pM.StaticLastSelection) >= pM.StaticSelectionInterval { // static selection
		pM.StaticLastSelection = time.Now()
		ps := pM.Pather.Paths(remoteAddr.IA)
		S := chooseNewPaths(ps, k)
		S_active := pickRandom(S, cap)
		pM.S = S
		pM.S_Active = S_active
		return S_active
	} else if time.Since(pM.StaticLastSelection) >= pM.WarmupPhase || time.Since(pM.DynamicLastSelection) >= pM.DynamicSelectionInterval { // After warmup phase or once an hour, do dynamic selection
		// do dynamic
		//
	}

	return pM.S_Active

}
*/

func (pM *PathManager) RunStaticSelection(ctx context.Context, log *slog.Logger) {
	ps := pM.Pather.Paths(pM.RemoteAddr.IA)
	S := chooseNewPaths(ps, pM.K)
	S_active := pickRandom(S, pM.Cap)
	pM.S = S
	pM.S_Active = S_active
	pM.assignProbers()
	log.Info("Static path selection completed", slog.Int("S_total", len(S)), slog.Int("S_active", len(S_active)))
}

func (pM *PathManager) RunDynamicSelection(ctx context.Context, log *slog.Logger) {
	// TODO: implement dynamic selection logic here, e.g., based on symmetry, jitter etc
	pM.probePaths(ctx)
	log.Info("Dynamic path selection completed (placeholder)")
}

// -------------------dynamic----------------------------

func (pM *PathManager) probePaths(ctx context.Context) {
	pathMap := make(map[string]snet.Path)
	for _, path := range pM.S {
		fp := snet.Fingerprint(path).String()
		pathMap[fp] = path
	}

	mtrcs := scionMetrics.Load()
	perProberTimestamps := make([][]TimeStamps, len(pM.Probers))

	for i, prober := range pM.Probers {

		if prober.InterleavedModePath() != "" && pathMap[prober.prev.path] != nil { // TODO: CHECK IT OUT

			go func(i int, ctx context.Context, log *slog.Logger, mtrcs *scionClientMetrics, prober *SCIONClient, p snet.Path) {
				var results []TimeStamps

				for j := 0; j < 20; j++ {
					_, _, e, timestamps := prober.getTimestamps(ctx, mtrcs, pM.LocalAddr, pM.RemoteAddr, p)
					if e != nil {
						log.LogAttrs(ctx, slog.LevelInfo, "failed to measure clock offset",
							slog.Any("to", pM.RemoteAddr),
							slog.Any("via", snet.Fingerprint(p).String()),
							slog.Any("error", e),
						)
						continue
					}
					results = append(results, timestamps)
					time.Sleep(1 * time.Second)
				}
				perProberTimestamps[i] = results
			}(i, ctx, prober.Log, mtrcs, prober, pathMap[prober.prev.path])
		}
	}

	for i, tsList := range perProberTimestamps {
		if tsList != nil {
			log := pM.Probers[i].Log
			log.LogAttrs(ctx, slog.LevelInfo, "finished probing path",
				slog.String("path", pM.Probers[i].InterleavedModePath()),
				slog.Int("samples", len(tsList)),
			)
		}
	}
}

// Each SCIONClient holds a path and we can probe the path with the help of this SCIONClient
// It is important to set SCIONClient c.InterleavedMode to false (default value), then the server will treat it as basic mode, not xleave mode
// The function below assigns each path of S to a SCIONClient. During probing, the SCIONClient has one path assigned which it will probe.
func (pM *PathManager) assignProbers() {
	sIndex := 0
	for _, prober := range pM.Probers {
		if prober == nil {
			continue
		}
		prober.ResetInterleavedMode() // Always reset

		if sIndex < len(pM.S) {
			selectedPath := pM.S[sIndex]
			prober.prev.path = snet.Fingerprint(selectedPath).String()
			sIndex++
		} else {
			prober.prev.path = "" // No path left to assign
		}
	}
}

// -------------------static----------------------------

func chooseNewPaths(availablePaths []snet.Path, numPaths int) []snet.Path {
	ch := make(chan int, 1)
	timeout, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	var computedPathSet []snet.Path

	go func() {
		computedPathSet = greedyDisjointPathSelection(availablePaths, len(availablePaths), numPaths)
		ch <- 1
	}()

	log := slog.Default()
	select {
	case <-timeout.Done():
		log.Warn(fmt.Sprintf("Path selection took too long! Using first few paths"))
		return availablePaths[:min(len(availablePaths), numPaths)]
	case <-ch:
		return computedPathSet
	}
}

func greedyDisjointPathSelection(paths []snet.Path, nbOfPaths int, k int) []snet.Path {
	if len(paths) <= k || len(paths) == 0 {
		return paths
	}

	selected := []snet.Path{}
	usedInterfaces := map[snet.PathInterface]int{}

	// Step 1: Pick initial path
	shortestLength := len(paths[0].Metadata().Interfaces)
	for _, p := range paths[1:] {
		if len(p.Metadata().Interfaces) < shortestLength {
			shortestLength = len(p.Metadata().Interfaces)
		}
	}
	shortestPaths := []snet.Path{}
	for _, p := range paths {
		if len(p.Metadata().Interfaces) == shortestLength {
			shortestPaths = append(shortestPaths, p)
		}
	}
	best := shortestPaths[secureRandomIndex(len(shortestPaths))]
	selected = append(selected, best)
	for _, iface := range best.Metadata().Interfaces {
		usedInterfaces[iface]++
	}

	// Step 2: Greedily add most disjoint paths
	for len(selected) < k {
		var nextBest snet.Path
		bestScore := math.MinInt
		bestHopCount := math.MaxInt
		for _, candidate := range paths {
			if alreadySelected(candidate, selected) {
				continue
			}

			score := disjointnessScore(candidate, usedInterfaces)
			hopCount := len(candidate.Metadata().Interfaces)
			if score > bestScore || (score == bestScore && hopCount < bestHopCount) { // second part is about prefering paths that are shorter
				nextBest = candidate
				bestScore = score
				bestHopCount = hopCount
			}
		}

		if nextBest != nil {
			selected = append(selected, nextBest)
			for _, iface := range nextBest.Metadata().Interfaces {
				usedInterfaces[iface]++
			}
		} else {
			break
		}

	}

	return selected
}

func secureRandomIndex(n int) int {
	if n <= 0 {
		return 0
	}
	max := big.NewInt(int64(n))
	i, err := rand.Int(rand.Reader, max)
	if err != nil {
		return 0
	}
	return int(i.Int64())
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func alreadySelected(p snet.Path, selected []snet.Path) bool {
	for _, s := range selected {
		if snet.Fingerprint(s) == snet.Fingerprint(p) {
			return true
		}
	}
	return false
}

func disjointnessScore(p snet.Path, used map[snet.PathInterface]int) int {
	score := 0
	for _, iface := range p.Metadata().Interfaces {
		score -= used[iface]
	}
	// Zero score = fully disjoint.
	// Negative score = many shared interfaces.
	return score
}

func combinedScore(p snet.Path, used map[snet.PathInterface]int, S []snet.Path, N []snet.Path) float64 {
	scoreDis := 0
	for _, iface := range p.Metadata().Interfaces {
		scoreDis -= used[iface]
	}

	sizeOfS := len(S)
	if sizeOfS == 0 || len(p.Metadata().Interfaces) == 0 {
		return 0 // avoid division by zero
	}
	normalizedOverlap := float64(scoreDis) / float64(len(p.Metadata().Interfaces)*sizeOfS)

	// Compute normalizedLen
	remainingPaths := removePaths(S, N) // N \ S
	if len(remainingPaths) == 0 {
		return normalizedOverlap // fallback score
	}

	shortestLen := float64(findShortestPath(remainingPaths))
	longestLen := float64(findLongestPath(remainingPaths))

	currLen := float64(len(p.Metadata().Interfaces))

	var normalizedLen float64
	if longestLen != shortestLen {
		normalizedLen = (currLen - shortestLen) / (longestLen - shortestLen)
	} else {
		normalizedLen = 0 // avoid division by zero if all lengths are equal
	}

	alpha := 0.5
	beta := 0.5
	score := alpha*normalizedLen + beta*normalizedOverlap

	return score
}

func removePaths(A, B []snet.Path) []snet.Path { // N\S
	// Collect fingerprints of paths in A
	aFingerprints := []string{}
	for _, a := range A {
		aFingerprints = append(aFingerprints, snet.Fingerprint(a).String())
	}

	// Filter B to exclude paths that are in A
	result := []snet.Path{}
	for _, b := range B {
		found := false
		for _, fp := range aFingerprints {
			if snet.Fingerprint(b).String() == fp {
				found = true
				break
			}
		}
		if !found {
			result = append(result, b)
		}
	}

	return result
}

func findShortestPath(paths []snet.Path) int {
	if len(paths) == 0 {
		return 0 // No paths, return zero value and false
	}

	// catch if len(paths) == 1!

	shortestLength := len(paths[0].Metadata().Interfaces)
	for _, p := range paths[1:] {
		if len(p.Metadata().Interfaces) < shortestLength {
			shortestLength = len(p.Metadata().Interfaces)
		}
	}
	return shortestLength
}

func findLongestPath(paths []snet.Path) int {
	if len(paths) == 0 {
		return 0 // No paths, return zero value and false
	}

	// catch if len(paths) == 1!

	longestLength := len(paths[0].Metadata().Interfaces)
	for _, p := range paths[1:] {
		if len(p.Metadata().Interfaces) > longestLength {
			longestLength = len(p.Metadata().Interfaces)
		}
	}
	return longestLength
}

func pickRandom(paths []snet.Path, cap int) []snet.Path {
	if cap >= len(paths) {
		return paths
	}

	remaining := make([]snet.Path, len(paths))
	copy(remaining, paths)

	selected := make([]snet.Path, 0, cap)

	for i := 0; i < cap; i++ {
		idx := secureRandomIndex(len(remaining))
		selected = append(selected, remaining[idx])

		// Remove the selected path from remaining (to avoid duplicates)
		remaining = append(remaining[:idx], remaining[idx+1:]...)
	}

	return selected

}

// -------------------not scalable-----------------------

type PathPickDescriptor struct {
	ruleIndex int
	pathIndex int
}

type PathPicker struct {
	pathSpec        *[]PathSpec
	availablePaths  []snet.Path
	currentPathPick []PathPickDescriptor
}

type PathInterface struct {
	ia   addr.IA
	ifId uint64
}

func (iface *PathInterface) ID() uint64 {
	return iface.ifId
}

func (iface *PathInterface) IA() addr.IA {
	return iface.ia
}

type PathSpec []PathInterface

type AppPathSet map[snet.PathFingerprint]snet.Path

func makePathPicker(pathSet []snet.Path, numPaths int) *PathPicker {
	paths := make([]snet.Path, 0, len(pathSet))
	for _, path := range pathSet {
		paths = append(paths, path)
	}
	picker := &PathPicker{
		availablePaths: paths,
	}
	picker.reset(numPaths)
	return picker
}

func (picker *PathPicker) reset(numPaths int) {
	descriptor := make([]PathPickDescriptor, numPaths)
	for i := range descriptor {
		descriptor[i].ruleIndex = -1
		descriptor[i].pathIndex = -1
	}
	picker.currentPathPick = descriptor
}

func (picker *PathPicker) disjointnessScore() int {
	interfaces := map[snet.PathInterface]int{}
	score := 0
	for _, pick := range picker.currentPathPick {
		for _, path := range picker.availablePaths[pick.pathIndex].Metadata().Interfaces {
			score -= interfaces[path]
			interfaces[path]++
		}
	}
	return score
}

func (picker *PathPicker) nextPick() bool {
	return picker.nextPickIterate(len(picker.currentPathPick) - 1)
}

func (picker *PathPicker) nextPickIterate(idx int) bool {
	if idx > 0 && picker.currentPathPick[idx-1].pathIndex == -1 {
		if !picker.nextPickIterate(idx - 1) {
			return false
		}
	}
	for true {
		for pathIdx := picker.currentPathPick[idx].pathIndex + 1; pathIdx < len(picker.availablePaths); pathIdx++ {
			if !picker.isInUse(pathIdx, idx) && picker.matches(pathIdx, picker.currentPathPick[idx].ruleIndex) {
				picker.currentPathPick[idx].pathIndex = pathIdx
				return true
			}
		}
		// overflow
		if idx > 0 {
			picker.currentPathPick[idx].pathIndex = -1
			if !picker.nextPickIterate(idx - 1) {
				return false
			}
		} else {
			break // cannot overflow, abort
		}
	}
	return false
}

func (iface *PathInterface) match(pathIface snet.PathInterface) bool {
	if iface.ifId == 0 {
		return iface.IA() == pathIface.IA
	}
	return iface.ID() == uint64(pathIface.ID) && iface.IA() == pathIface.IA
}

func (picker *PathPicker) matches(pathIdx, ruleIdx int) bool {
	return true
}

func (picker *PathPicker) isInUse(pathIdx, idx int) bool {
	for i, pick := range picker.currentPathPick {
		if i > idx {
			return false
		}
		if pick.pathIndex == pathIdx {
			return true
		}
	}
	return false
}

func (picker *PathPicker) nextRuleSet() bool {
	if picker.currentPathPick[0].ruleIndex == -1 {
		for i := range picker.currentPathPick {
			picker.currentPathPick[i].ruleIndex = 0
			picker.currentPathPick[i].pathIndex = -1
		}
		return true
	}
	return false
}

func (picker *PathPicker) maxRuleIdx() int {
	// rule indices are sorted ascending
	for idx := len(picker.currentPathPick) - 1; idx >= 0; idx++ {
		if picker.currentPathPick[idx].ruleIndex != -1 {
			return picker.currentPathPick[idx].ruleIndex
		}
	}
	return -1
}

func (picker *PathPicker) getPaths() []snet.Path {
	paths := make([]snet.Path, 0, len(picker.currentPathPick))
	for _, pick := range picker.currentPathPick {
		paths = append(paths, picker.availablePaths[pick.pathIndex])
	}
	return paths
}

func ChooseNewPaths_notscalable(availablePaths []snet.Path, numPaths int) []snet.Path {
	// Because this path selection takes too long when many paths are available
	// (tens of seconds), we run it with a timeout and fall back to using the
	// first few paths if it takes too long.
	ch := make(chan int, 1)
	timeout, cancel := context.WithTimeout(context.Background(), 55*time.Minute)
	defer cancel()

	var computedPathSet []snet.Path
	go func() { // pick paths
		picker := makePathPicker(availablePaths, numPaths)
		disjointness := 0 // negative number denoting how many network interfaces are shared among paths (to be maximized)
		for i := numPaths; i > 0; i-- {
			picker.reset(i)
			for picker.nextPick() { // iterate through different choices of paths obeying the rules of the current set of PathSpecs
				curDisjointness := picker.disjointnessScore()
				if computedPathSet == nil || disjointness < curDisjointness { // maximize disjointness
					disjointness = curDisjointness
					computedPathSet = picker.getPaths()
				}
			}
			if computedPathSet != nil { // if no path set of size i found, try with i-1
				break
			}
		}
		ch <- 1
	}()

	log := slog.Default()
	select {
	case <-timeout.Done():
		log.Warn(fmt.Sprintf("Path selection took too long! Using first few paths"))
		return availablePaths[:min(len(availablePaths), numPaths)]
	case <-ch:
		return computedPathSet
	}
}
