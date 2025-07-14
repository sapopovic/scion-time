package client

import (
	"context"
	"crypto/rand"
	"fmt"
	"log/slog"
	"math"
	"math/big"
	"sort"
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
	PingDuration             int
	MetricsPerProber         map[int]*PathMetrics
}

type ProbeResult struct {
	Index          int
	Timestamps     []TimeStamps
	SuccessCount   int
	AttemptedCount int
}

type PathScore struct {
	Index       int
	Path        string
	Q           float64
	Symmetry    float64
	MinRTT      float64
	Jitter      float64
	SuccessRate float64
}

type MetricEMA struct {
	Value   float64
	HasPrev bool
}

type PathMetrics struct {
	MinRTT      float64
	JitterEMA   MetricEMA
	AsymEMA     MetricEMA
	QScoreEMA   MetricEMA
	SampleCount int
	LossCount   int
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
	pM.MetricsPerProber = make(map[int]*PathMetrics)
	S := chooseNewPaths(ps, pM.K)
	S_active := pickRandom(S, pM.Cap)
	pM.S = S
	pM.S_Active = S_active
	pM.assignProbers()

	// for _, prober := range pM.Probers {
	// 	log.LogAttrs(ctx, slog.LevelInfo, "HIIII", slog.Any("xleave?", prober.InterleavedMode), slog.Any("prev struct", prober.prev))
	// }

	log.Info("Static path selection completed", slog.Int("S_total", len(S)), slog.Int("S_active", len(S_active)))
}

func (pM *PathManager) RunDynamicSelection(ctx context.Context, log *slog.Logger) {
	pM.probePaths(ctx, log) // Updates PathMetrics for each path with EVERY NEW MEASUREMENT. These are performance results.
	pM.PrintSortedPathsByQ(log)
}

// -------------------dynamic----------------------------

/*
func (pM PathManager) analyzeProbes(ctx context.Context, results []ProbeResult, log *slog.Logger) {

	var pathScores []PathScore

	for i, res := range results {
		tsList := res.Timestamps
		if tsList == nil || len(tsList) == 0 {
			log.LogAttrs(ctx, slog.LevelInfo, "No responses for prober",
				slog.Int("prober", i),
				slog.Int("attempted", res.AttemptedCount),
				slog.Int("successful", res.SuccessCount),
			)
			continue
		}

		log.LogAttrs(ctx, slog.LevelInfo, "Probe response summary",
			slog.Int("prober", i),
			slog.Int("attempted", res.AttemptedCount),
			slog.Int("successful", res.SuccessCount),
			slog.Float64("success_rate", float64(res.SuccessCount)/float64(res.AttemptedCount)),
		)

		var symmetryVals []float64
		var rttVals []float64
		minRTT := math.MaxFloat64

		for _, ts := range tsList {
			if ts.t0.IsZero() || ts.t1.IsZero() || ts.t2.IsZero() || ts.t3.IsZero() || ts.t3.Before(ts.t2) || ts.t2.Before(ts.t1) || ts.t1.Before(ts.t0) {
				continue // skip invalid timestamps
			}

			d1 := ts.t1.Sub(ts.t0).Seconds()
			d2 := ts.t3.Sub(ts.t2).Seconds()
			symmetry := math.Abs(d1 - d2)
			rtt := d1 + d2

			symmetryVals = append(symmetryVals, symmetry)
			rttVals = append(rttVals, rtt)

			if rtt < minRTT {
				minRTT = rtt
			}
		}

		avgSym := avg(symmetryVals)
		jitter := stddev(rttVals)
		successRate := float64(res.SuccessCount) / float64(res.AttemptedCount)

		// small coefficients in norm are too aggressive
		// symNorm := normalize(avgSym, 0.02) // 20ms scale
		// minRTTNorm := normalize(minRTT, 0.05) // 50ms baseline
		// jitterNorm := normalize(jitter, 0.03) // 30ms jitter scale
		// lossNorm := 1 - successRate                         // higher = worse
		// combinedRTTScore := 0.6*minRTTNorm + 0.4*jitterNorm // jitter higher because of accuracy NTP!

		// Emphasize on symmetry, then stability (jitter), then baseline latency, then availability
		// Q := 0.5*symNorm + 0.4*combinedRTTScore + 0.1*lossNorm
		Q := avgSym

		path := ""
		if i < len(pM.Probers) && pM.Probers[i] != nil {
			path = pM.Probers[i].prev.path
		}
		pathScores = append(pathScores, PathScore{
			Index:       i,
			Path:        path,
			Q:           Q,
			Symmetry:    avgSym,
			MinRTT:      minRTT,
			Jitter:      jitter,
			SuccessRate: successRate,
		})
		// Q = w_sym * norm(symmetry) + w_rtt * combinedRTTScore + w_loss * (1 - successRate) | combinedRTTScore = combination minRTT & jitter
	}

	sort.Slice(pathScores, func(i, j int) bool {
		return pathScores[i].Q < pathScores[j].Q
	})
	log.Info("Sorted paths by Q (lower is better):")
	for _, ps := range pathScores {
		// log.LogAttrs(ctx, slog.LevelInfo, "Path score",
		// 	slog.Int("prober", ps.Index),
		// 	slog.String("path", ps.Path),
		// 	slog.Float64("Q", ps.Q),
		// 	slog.Float64("symmetry", ps.Symmetry),
		// 	slog.Float64("min_rtt", ps.MinRTT),
		// 	slog.Float64("jitter", ps.Jitter),
		// 	slog.Float64("success_rate", ps.SuccessRate),
		// )
		log.LogAttrs(ctx, slog.LevelInfo, "Path score",
			slog.Int("prober", ps.Index),
			slog.Float64("Q", ps.Q),
		)
	}
}*/

func (pM *PathManager) PrintSortedPathsByQ(log *slog.Logger) {
	type ranked struct {
		Index   int
		Metrics *PathMetrics
	}

	var list []ranked
	for index, metrics := range pM.MetricsPerProber {
		if metrics.SampleCount == 0 {
			continue // Skip uninitialized paths
		}
		list = append(list, ranked{Index: index, Metrics: metrics})
	}

	// Sort in ascending order of QScoreEMA.Value
	sort.Slice(list, func(i, j int) bool {
		return list[i].Metrics.QScoreEMA.Value < list[j].Metrics.QScoreEMA.Value
	})

	// Print sorted path metrics
	for _, entry := range list {
		m := entry.Metrics
		log.Info("Path score",
			slog.Int("prober", entry.Index),
			slog.Float64("Q", m.QScoreEMA.Value),
			slog.Float64("jitter", m.JitterEMA.Value),
			slog.Float64("asymmetry", m.AsymEMA.Value),
			slog.Float64("minRTT", m.MinRTT),
			slog.Int("samples", m.SampleCount),
			slog.Int("losses", m.LossCount),
		)
	}
}

func normalize(x, scale float64) float64 {
	return x / (x + scale) // if scale is big, then graph is grow very slowly. if the scale is small, the slope is steep.
}

func avg(xs []float64) float64 {
	if len(xs) == 0 {
		return 0
	}
	sum := 0.0
	for _, x := range xs {
		sum += x
	}
	return sum / float64(len(xs))
}

func stddev(xs []float64) float64 {
	if len(xs) == 0 {
		return 0
	}
	mean := avg(xs)
	variance := 0.0
	for _, x := range xs {
		d := x - mean
		variance += d * d
	}
	return math.Sqrt(variance / float64(len(xs)))
}

func updateEMA(metric *MetricEMA, newVal, baseAlpha, maxAlpha, frac float64) {
	if !metric.HasPrev {
		metric.Value = newVal
		metric.HasPrev = true
		return
	}
	delta := math.Abs(newVal - metric.Value)
	scale := 1.0 / (frac * metric.Value)
	alpha := baseAlpha + math.Min(delta*scale, maxAlpha-baseAlpha)
	metric.Value = alpha*newVal + (1-alpha)*metric.Value
}

func (pM *PathManager) probePaths(ctx context.Context, log *slog.Logger) {
	pathMap := make(map[string]snet.Path)
	for _, path := range pM.S {
		fp := snet.Fingerprint(path).String()
		pathMap[fp] = path
	}

	mtrcs := scionMetrics.Load()

	nProbers := 0

	for i, prober := range pM.Probers {
		if prober.prev.path != "" {
			if path, ok := pathMap[prober.prev.path]; ok {
				nProbers++
				go func(i int, prober *SCIONClient, p snet.Path) {

					if _, ok := pM.MetricsPerProber[i]; !ok {
						pM.MetricsPerProber[i] = &PathMetrics{MinRTT: math.MaxFloat64}
					}
					metrics := pM.MetricsPerProber[i]

					for j := 0; j < pM.PingDuration; j++ {
						pingCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
						_, _, e, timestamps := prober.getTimestamps(pingCtx, mtrcs, pM.LocalAddr, pM.RemoteAddr, p)
						cancel()

						if e != nil {
							prober.Log.LogAttrs(ctx, slog.LevelInfo, "Timeout or error during probing",
								slog.Any("to", pM.RemoteAddr),
								slog.Any("via", snet.Fingerprint(p).String()),
								slog.Any("error", e),
							)
							metrics.LossCount++
							continue
						}

						if timestamps.t0.IsZero() || timestamps.t1.IsZero() || timestamps.t2.IsZero() || timestamps.t3.IsZero() || timestamps.t3.Before(timestamps.t2) || timestamps.t2.Before(timestamps.t1) || timestamps.t1.Before(timestamps.t0) {
							continue // skip invalid timestamps
						}

						d1 := timestamps.t1.Sub(timestamps.t0).Seconds()
						d2 := timestamps.t3.Sub(timestamps.t2).Seconds()
						rtt := d1 + d2

						asym := math.Abs(d1 - d2)

						if rtt < metrics.MinRTT { // TODO: look at that again
							metrics.MinRTT = rtt
						}

						// jitter := math.Abs(rtt - metrics.MinRTT)
						// updateEMA(&metrics.JitterEMA, jitter, 0.3, 0.8, 0.001) // frac value TO BE CHANGED
						// updateEMA(&metrics.AsymEMA, asym, 0.3, 0.8, 0.001) // frac value 0.1 TO BE CHANGED

						// jitterNorm := normalize(metrics.JitterEMA.Value, 0.01) // ADD LATER
						// asymNorm := normalize(metrics.AsymEMA.Value, 0.0005) // asymmetry jumps around 500 microseconds, frac value TO BE CHANGED

						// Q := 0.6*asymNorm + 0.4*jitterNorm // + 0.2*lossNorm
						Q := asym
						// updateEMA(&metrics.QScoreEMA, Q, 0.3, 0.8, 0.1)
						updateEMA(&metrics.QScoreEMA, Q, 0.3, 0.8, 0.001)

						metrics.SampleCount++
						time.Sleep(1 * time.Second)
					}
				}(i, prober, path)
			}
		}
	}
}

func (ts TimeStamps) String() string {
	return fmt.Sprintf("t0=%s, t1=%s, t2=%s, t3=%s",
		ts.t0.Format(time.RFC3339Nano),
		ts.t1.Format(time.RFC3339Nano),
		ts.t2.Format(time.RFC3339Nano),
		ts.t3.Format(time.RFC3339Nano),
	)
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
