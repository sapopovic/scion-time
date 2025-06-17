package client

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"sync/atomic"
	"time"

	"github.com/scionproto/scion/pkg/addr"
	"github.com/scionproto/scion/pkg/snet"

	"example.com/scion-time/base/crypto"

	"example.com/scion-time/core/measurements"

	"example.com/scion-time/net/udp"
)

type ReferenceClock interface {
	MeasureClockOffset(ctx context.Context) (time.Time, time.Duration, error)
}

type ReferenceClockClient struct {
	numOpsInProgress uint32
}

var (
	errNoPath             = errors.New("failed to measure clock offset: no path")
	errUnexpectedAddrType = errors.New("unexpected address type")

	ipMetrics    atomic.Pointer[ipClientMetrics]
	scionMetrics atomic.Pointer[scionClientMetrics]
)

func init() {
	ipMetrics.Store(newIPClientMetrics())
	scionMetrics.Store(newSCIONClientMetrics())
}

func MeasureClockOffsetIP(ctx context.Context, log *slog.Logger,
	ntpc *IPClient, localAddr, remoteAddr *net.UDPAddr) (
	ts time.Time, off time.Duration, err error) {
	mtrcs := ipMetrics.Load()

	var nerr, n int
	log.LogAttrs(ctx, slog.LevelDebug, "measuring clock offset",
		slog.Any("to", remoteAddr),
	)
	if ntpc.InterleavedMode {
		n = 3
	} else {
		n = 1
	}
	for i := range n {
		t, o, e := ntpc.measureClockOffsetIP(ctx, mtrcs, localAddr, remoteAddr)
		if e == nil {
			ts, off, err = t, o, e
			if ntpc.InInterleavedMode() {
				break
			}
		} else {
			if nerr == i {
				err = e
			}
			nerr++
			log.LogAttrs(ctx, slog.LevelInfo, "failed to measure clock offset",
				slog.Any("to", remoteAddr),
				slog.Any("error", e),
			)
		}
	}
	return
}

func collectMeasurements(ctx context.Context, ms []measurements.Measurement, msc chan measurements.Measurement) int {
	i := 0
	j := 0
	n := len(ms)
loop:
	for i != n {
		select {
		case m := <-msc:
			if m.Error == nil {
				if j != len(ms) {
					ms[j] = m
					j++
				}
			}
			i++
		case <-ctx.Done():
			break loop
		}
	}
	go func(n int) { // drain channel
		for n != 0 {
			<-msc
			n--
		}
	}(n - i)
	return j
}

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

func ChooseNewPaths(availablePaths []snet.Path, numPaths int) []snet.Path {
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

func MeasureClockOffsetSCION(ctx context.Context, log *slog.Logger,
	ntpcs []*SCIONClient, localAddr, remoteAddr udp.UDPAddr, ps []snet.Path, chosenPaths []string, selectionMethod string) (
	time.Time, time.Duration, error) {
	mtrcs := scionMetrics.Load()

	sps := make([]snet.Path, len(ntpcs))
	nsps := 0

	if chosenPaths != nil {
		for _, chosenPath := range chosenPaths { // iterate through fingerprints
			for i, p := range ps {
				pf := snet.Fingerprint(p).String()
				if pf == chosenPath {
					sps[i] = p
					nsps++
					break
				}
			}
		}
	} else {
		for i, c := range ntpcs {
			pf := c.InterleavedModePath()
			if pf != "" {
				for j := range len(ps) {
					if p := ps[j]; snet.Fingerprint(p).String() == pf {
						ps[j] = ps[len(ps)-1]
						ps = ps[:len(ps)-1]
						sps[i] = p
						nsps++
						break
					}
				}
			}
			if sps[i] == nil {
				c.ResetInterleavedMode()
				if c.Filter != nil {
					c.Filter.Reset()
				}
			}
		}
		n, err := crypto.Sample(ctx, len(sps)-nsps, len(ps), func(dst, src int) {
			ps[dst] = ps[src]
		})
		if err != nil {
			return time.Time{}, 0, err
		}
		if nsps+n == 0 {
			return time.Time{}, 0, errNoPath
		}
		for i, j := 0, 0; j != n; j++ {
			for sps[i] != nil {
				i++
			}
			sps[i] = ps[j]
			nsps++
		}
	}

	ms := make([]measurements.Measurement, nsps)
	msc := make(chan measurements.Measurement)
	for i := range len(ntpcs) {
		if sps[i] == nil {
			continue
		}
		go func(ctx context.Context, log *slog.Logger, mtrcs *scionClientMetrics,
			ntpc *SCIONClient, localAddr, remoteAddr udp.UDPAddr, p snet.Path) {
			var err error
			var ts time.Time
			var off time.Duration
			var nerr, n int
			log.LogAttrs(ctx, slog.LevelDebug, "measuring clock offset",
				slog.Any("to", remoteAddr),
				slog.Any("via", snet.Fingerprint(p).String()),
				slog.Any("path", p),
			)
			if ntpc.InterleavedMode {
				n = 3
			} else {
				n = 1
			}
			for j := range n {
				t, o, e := ntpc.measureClockOffsetSCION(ctx, mtrcs, localAddr, remoteAddr, p)
				if e == nil {
					ts, off, err = t, o, e
					if ntpc.InInterleavedMode() {
						break
					}
				} else {
					if nerr == j {
						err = e
					}
					nerr++
					log.LogAttrs(ctx, slog.LevelInfo, "failed to measure clock offset",
						slog.Any("to", remoteAddr),
						slog.Any("via", snet.Fingerprint(p).String()),
						slog.Any("error", e),
					)
				}
			}
			msc <- measurements.Measurement{
				Timestamp: ts,
				Offset:    off,
				Error:     err,
			}
		}(ctx, log, mtrcs, ntpcs[i], localAddr, remoteAddr, sps[i])
	}
	collectMeasurements(ctx, ms, msc)
	log.LogAttrs(ctx, slog.LevelInfo, "Merge offsets with the following", slog.Any("selection method", selectionMethod))
	m := measurements.SelectMethod(ms, selectionMethod)
	return m.Timestamp, m.Offset, m.Error
}

func (c *ReferenceClockClient) MeasureClockOffsets(ctx context.Context,
	refclks []ReferenceClock, ms []measurements.Measurement) {
	if len(ms) != len(refclks) {
		panic("number of result offsets must be equal to the number of reference clocks")
	}
	swapped := atomic.CompareAndSwapUint32(&c.numOpsInProgress, 0, 1)
	if !swapped {
		panic("too many reference clock offset measurements in progress")
	}
	defer func(addr *uint32) {
		swapped := atomic.CompareAndSwapUint32(addr, 1, 0)
		if !swapped {
			panic("inconsistent count of reference clock offset measurements")
		}
	}(&c.numOpsInProgress)

	msc := make(chan measurements.Measurement)
	for _, refclk := range refclks {
		go func(ctx context.Context, refclk ReferenceClock) {
			ts, off, err := refclk.MeasureClockOffset(ctx)
			msc <- measurements.Measurement{
				Timestamp: ts,
				Offset:    off,
				Error:     err,
			}
		}(ctx, refclk)
	}
	collectMeasurements(ctx, ms, msc)
}
