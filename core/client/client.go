package client

import (
	"context"
	"errors"
	"log/slog"
	"net"
	"sync/atomic"
	"time"

	"github.com/scionproto/scion/pkg/snet"

	"example.com/scion-time/base/crypto"

	"example.com/scion-time/core/measurements"
	"example.com/scion-time/core/timebase"

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
	ts := timebase.Now()
	for j != len(ms) {
		ms[j] = measurements.Measurement{Timestamp: ts}
		j++
	}
	go func(n int) { // drain channel
		for n != 0 {
			<-msc
			n--
		}
	}(n - i)
	return j
}

func MeasureClockOffsetSCION_v2(ctx context.Context, log *slog.Logger,
	ntpcs []*SCIONClient, sps []snet.Path, localAddr, remoteAddr udp.UDPAddr) (time.Time, time.Duration, error) {
	mtrcs := scionMetrics.Load()

	// IDEA: if after static selection or dynamic selection some paths remain the same, then wen don't want to throw away the filter.

	// file, err := os.OpenFile("output.txt", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	// if err != nil {
	// 	return time.Time{}, 0, fmt.Errorf("failed to open file: %w", err)
	// }
	// var wg sync.WaitGroup
	// var mu sync.Mutex

	fpToClient := make(map[string]*SCIONClient) // Get all fingerprints from SCIONClients

	// Step 1: Extract fingerprint from SCIONClient
	for _, client := range ntpcs {
		fp := client.InterleavedModePath()
		if fp != "" {
			fpToClient[fp] = client
		}
	}

	assigned := make(map[*SCIONClient]bool)
	reorderedNTPCs := make([]*SCIONClient, len(sps)) // Map SCIONClient to path in sps (same indices)

	// Step 2: First pass -> match paths to known clients
	for i, path := range sps {
		fp := snet.Fingerprint(path).String()
		if client, ok := fpToClient[fp]; ok && !assigned[client] { // We found a SCIONClient that has the fingerprint
			reorderedNTPCs[i] = client // Assign SCIONClient to path i of sps
			assigned[client] = true
		} // if no match, then reorderedNTPCs[i] = nil automatically
	}

	// Step 3: Second pass -> assign unmatched paths to any free clients
	for i, client := range reorderedNTPCs { // Example [c3 nil c4 c1 c2 nil ...]
		if client != nil {
			continue
		}
		for _, c := range ntpcs { // Pick a free SCIONClient
			if !assigned[c] {
				reorderedNTPCs[i] = c
				assigned[c] = true
				c.ResetInterleavedMode()
				if c.Filter != nil {
					c.Filter.Reset()
				} // New fingerprint is set in client_scion.go upon first NTP measurement
				break
			}
		}
	}

	ms := make([]measurements.Measurement, len(ntpcs))
	msc := make(chan measurements.Measurement)
	for i := range len(sps) {
		// wg.Add(1)
		go func(ctx context.Context, log *slog.Logger, mtrcs *scionClientMetrics, ntpc *SCIONClient, localAddr, remoteAddr udp.UDPAddr, p snet.Path) {
			// defer wg.Done()
			var err error
			var ts time.Time
			var off time.Duration
			var nerr, n int
			// log.LogAttrs(ctx, slog.LevelDebug, "measuring clock offset",
			// 	slog.Any("to", remoteAddr),
			// 	slog.Any("via", snet.Fingerprint(p).String()),
			// 	slog.Any("path", p),
			// )
			log.LogAttrs(ctx, slog.LevelDebug, "SCIONClient <-> SPS", slog.Any("ntpcs", ntpc.InterleavedModePath()), slog.Any("sps", snet.Fingerprint(p).String()))
			// mu.Lock()
			// fmt.Fprintf(file, "Time: %s | Path: %s | SCIONClient: %s\n", time.Now().Format(time.RFC3339), snet.Fingerprint(p).String(), ntpc.InterleavedModePath())
			// mu.Unlock()
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
	// wg.Wait()

	collectMeasurements(ctx, ms, msc)
	// log.LogAttrs(ctx, slog.LevelInfo, "Merge offsets with the following", slog.Any("selection method", selectionMethod))
	m := measurements.SelectMethod(ms, "midpoint")
	// err = file.Close()
	// if err != nil {
	// 	log.Error("failed to close file", slog.Any("error", err))
	// }
	// log.LogAttrs(ctx, slog.LevelDebug, "Return Value", slog.Any("m.Timestamp", m.Timestamp), slog.Any("m.Offset", m.Offset), slog.Any("m.Error", m.Error), slog.Any("#measurements", len(ms)))
	return m.Timestamp, m.Offset, m.Error
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
