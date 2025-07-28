package client

import (
	"context"
	"fmt"
	"log/slog"
	"math/rand"
	"strconv"
	"strings"
	"time"

	"example.com/scion-time/base/logbase"
	"example.com/scion-time/driver/shm"
	"example.com/scion-time/net/ntp"
	"github.com/scionproto/scion/pkg/snet"
)

// We have one simulator per path that extracts information about the given path from the config
// The config will contain jitter, asymmetry information about each path.
// There will be multiple configs: high jitter/low asymmetry, high jitter/high asymmetry, path failure
// We have one simulator per path that extracts information about the given path from the config after time x
type Simulator struct {
	pathConfig    string
	rttMin        int
	rttMax        int
	asymRange     time.Duration
	log           *slog.Logger
	SHM           ReferenceClock
	pathQualities map[string]*PathQuality
	t3            time.Time
	off           time.Duration
}

type PathQuality struct {
	rttRange []float64  // [min, max] in milliseconds
	meanRTT  float64    // manually defined mean RTT
	jitter   float64    // standard deviation (jitter)
	rng      *rand.Rand // seeded RNG
}

func NewSimulator(simRefClock []string) *Simulator {
	log := slog.Default()
	refClock := make([]ReferenceClock, 0)

	for _, s := range simRefClock { // we only have one but we still iterate through a list (|list|=1) so that we preserve structure (to be safe)
		t := strings.Split(s, ":")
		if len(t) > 2 || t[0] != shm.ReferenceClockType {
			logbase.Fatal(slog.Default(), "unexpected SHM reference clock id", slog.String("id", s))
		}
		var u int
		if len(t) > 1 {
			var err error
			u, err = strconv.Atoi(t[1])
			if err != nil {
				logbase.Fatal(slog.Default(), "unexpected SHM reference clock id",
					slog.String("id", s), slog.Any("error", err))
			}
		}
		refClock = append(refClock, shm.NewReferenceClock(log, u)) // we only have 1 element
	}

	pqs := assignPathQualities_v1()

	return &Simulator{log: log, SHM: refClock[0], pathQualities: pqs}
}

func (s Simulator) generateTimeStamps(ctx context.Context, p snet.Path) TimeStamps {
	// Step 1: Fetch t3 and offset from SHM (or use fixed for simulation)
	// t3, off := s.t3, s.off // GNSS-based local time and offset
	// Test 2: The only unknown is the drift
	t3, off := time.Now(), time.Duration(0)

	// Step 2: Retrieve path-specific configuration
	pq := s.pathQualities[snet.Fingerprint(p).String()]
	baseRTT := pq.rttRange[0]

	// Step 3: Define fixed baseline asymmetry (e.g., 0.4/0.6)
	// UPLINK SLOWER IN OUR NETWORK
	d0Base := 0.55 * baseRTT
	d1Base := 0.45 * baseRTT

	// Step 4: Sample RTT from clamped normal distribution
	sampledRTT := float64(sampleClampedNormal(
		pq.rng,
		pq.meanRTT,
		pq.jitter,
		pq.rttRange[0],
		pq.rttRange[1],
	))
	delta := sampledRTT - baseRTT

	// UPLINK SLOWER IN OUR NETWORK
	// Step 5: Apply asymmetry bias with small jitter around alpha = 0.75
	// base delays are already a bit asymmetric, but, when rtt>minrtt, then we add more of the extra delay to the uplink
	baseAlpha := 0.75
	alpha := baseAlpha + 0.05*pq.rng.NormFloat64()
	if alpha < 0.5 { // Add to both but since base d0 > base d1 -> d0 will still be larger
		alpha = 0.5
	} else if alpha > 1.0 { // Add all only to uplink
		alpha = 1.0
	}

	// Step 6: Adjust d0 and d1 with delta
	d0, d1 := d0Base, d1Base
	if d0Base > d1Base { // base uplink base latency is bigger than base downlink latency
		d0 += alpha * delta // increase uplink more
		d1 += (1 - alpha) * delta
	} else {
		d0 += ((1 - alpha) * delta)
		d1 += alpha * delta
	}

	d0_duration := time.Duration(d0)
	d1_duration := time.Duration(d1)

	// Test 1: delays equal, so offsets must be equal
	// d0_duration = time.Duration(sampledRTT / 2.0)
	// d1_duration = time.Duration(sampledRTT / 2.0)
	// d0 = sampledRTT / 2.0
	// d1 = sampledRTT / 2.0

	// Step 7: Reconstruct timestamps
	t2 := t3.Add(-d1_duration).Add(off)  // GNSS time (server send)
	t1 := t2                             // server recv = server send
	t0 := t1.Add(-d0_duration).Add(-off) // client send (local clock)

	ts := TimeStamps{
		t0: t0,
		t1: t1,
		t2: t2,
		t3: t3,
	}

	// Step 8: Logging
	logMsg := fmt.Sprintf(
		"Generated timestamps: t0=%s, t1=%s, t2=%s, t3=%s | delays: d0=%v, d1=%v | RTT: %v | Test 2: SampledRTT?=d0+d1: %t | SHM offset: off=%v | NTP offset: off=%v | Test 1: %t",
		ts.t0.Format("15:04:05.000000"),
		ts.t1.Format("15:04:05.000000"),
		ts.t2.Format("15:04:05.000000"),
		ts.t3.Format("15:04:05.000000"),
		d0,
		d1,
		time.Duration(sampledRTT),
		(d0+d1) == sampledRTT,
		off,
		ntp.ClockOffset(t0, t1, t2, t3),
		off == ntp.ClockOffset(t0, t1, t2, t3),
	)
	fmt.Println(logMsg)

	return ts
}

// Sample from normal distribution, clamped to [min, max]
func sampleClampedNormal(rng *rand.Rand, mean, stdDev, min, max float64) time.Duration {
	for {
		sample := rng.NormFloat64()*stdDev + mean
		if sample >= min && sample <= max {
			return time.Duration(sample * float64(time.Millisecond))
		}
	}
}

// randomly_picked_paths := []string{"d169d3fdcf2a7b89091fb85c61222c3cb3fed169bfa3994a49891042917553e7", "5fb945f4d62fb9a9877489f10006d8f9c5a94f16656121e5eb78a0394a4f466e", "6c4f86d4c33b5c2371494582f3224b1a3c0611709168c4f411f66c9fa27d744b", "e1bf6b79f7219745133909e2a6fba731b92d001007cf81235cec08f3491bf196", "b1319d5e0f1cc40a0d227ef38e03ee6d58bd352393cb48f6fbeea0d29032b036", "a7a0a2901cccd754534d65374136b9829a7f81518309af79d87f0a560a14690c", "02bf95b27afd43858f75306b7a77c5d548ab4618a12cb155775ead71345028f9"}
func assignPathQualities_v1() map[string]*PathQuality {
	return map[string]*PathQuality{
		// --- Very bad paths (1–7): high RTT and jitter ---
		"d169d3fdcf2a7b89091fb85c61222c3cb3fed169bfa3994a49891042917553e7": {
			rttRange: []float64{50, 80},
			meanRTT:  75, jitter: 10, rng: rand.New(rand.NewSource(100)),
		}, // very bad

		"5fb945f4d62fb9a9877489f10006d8f9c5a94f16656121e5eb78a0394a4f466e": {
			rttRange: []float64{50, 80},
			meanRTT:  73, jitter: 9, rng: rand.New(rand.NewSource(101)),
		}, // very bad

		"6c4f86d4c33b5c2371494582f3224b1a3c0611709168c4f411f66c9fa27d744b": {
			rttRange: []float64{50, 80},
			meanRTT:  72, jitter: 8, rng: rand.New(rand.NewSource(102)),
		}, // very bad

		"e1bf6b79f7219745133909e2a6fba731b92d001007cf81235cec08f3491bf196": {
			rttRange: []float64{50, 80},
			meanRTT:  70, jitter: 9, rng: rand.New(rand.NewSource(103)),
		}, // very bad

		"b1319d5e0f1cc40a0d227ef38e03ee6d58bd352393cb48f6fbeea0d29032b036": {
			rttRange: []float64{50, 80},
			meanRTT:  71, jitter: 8, rng: rand.New(rand.NewSource(104)),
		}, // very bad

		"a7a0a2901cccd754534d65374136b9829a7f81518309af79d87f0a560a14690c": {
			rttRange: []float64{50, 80},
			meanRTT:  74, jitter: 10, rng: rand.New(rand.NewSource(105)),
		}, // very bad

		"02bf95b27afd43858f75306b7a77c5d548ab4618a12cb155775ead71345028f9": {
			rttRange: []float64{50, 80},
			meanRTT:  73, jitter: 10, rng: rand.New(rand.NewSource(106)),
		}, // very bad

		// --- Ranging from very good to bad ---
		"1c91e925e44aebc06938ee1f41ad22d0c2c3691a877eaf57eb974088bcbd2e4e": {
			rttRange: []float64{20, 30},
			meanRTT:  25, jitter: 1.5, rng: rand.New(rand.NewSource(107)),
		}, // very good

		"4194af709787d03483b2c8f298a1e12fba6abb384a62ded807cb4cb7ee197fd7": {
			rttRange: []float64{40, 80},
			meanRTT:  60, jitter: 6, rng: rand.New(rand.NewSource(108)),
		}, // mid

		"877b3a86f6e88d9034423fde74939b9362cbc8ac368cec0be7eab8e5a9663c6e": {
			rttRange: []float64{50, 60},
			meanRTT:  55, jitter: 3, rng: rand.New(rand.NewSource(109)),
		}, // mid

		"108acbc64efbf5a04652c15db371381c37fba6646fcc000bde7b7877cc8db557": {
			rttRange: []float64{10, 20},
			meanRTT:  15, jitter: 1, rng: rand.New(rand.NewSource(110)),
		}, // very good

		"3496ccc115ec697f3e2027c1f2b70a364bc08cf49ecb6fcb335c3989fca26b02": {
			rttRange: []float64{100, 150},
			meanRTT:  130, jitter: 12, rng: rand.New(rand.NewSource(111)),
		}, // very bad

		"1c1badde515e0cba50c1cbddeb792683884d22bcdfde8a6f4722d3be4dc9cb01": {
			rttRange: []float64{80, 90},
			meanRTT:  85, jitter: 5, rng: rand.New(rand.NewSource(112)),
		}, // bad

		"e488d5b91a7b360294f19bad4ddf644c41d7e47c82cb68256bbedc3eed9a6447": {
			rttRange: []float64{30, 40},
			meanRTT:  35, jitter: 1.5, rng: rand.New(rand.NewSource(113)),
		}, // good

		"aa170b5d350ffdd234572466a5b9e557c6930d2d8669a3f12a813c9410a0c0c7": {
			rttRange: []float64{20, 50},
			meanRTT:  40, jitter: 4, rng: rand.New(rand.NewSource(114)),
		}, // mid

		"c138518cd3637650efb49977186c920375cc6b36f75923be55d04f8f9303e188": {
			rttRange: []float64{10, 100},
			meanRTT:  70, jitter: 15, rng: rand.New(rand.NewSource(115)),
		}, // bad

		"0e8ebf758be09f95eb8f9f5df6eadad00cece2207a4ced88eac43dfafc68597f": {
			rttRange: []float64{22, 39},
			meanRTT:  30, jitter: 2, rng: rand.New(rand.NewSource(116)),
		}, // good

		"d8c9132fa9db80172bcd51547b87137986ee0533e49483ce4fcba9c277c26e4a": {
			rttRange: []float64{10, 15},
			meanRTT:  13, jitter: 0.8, rng: rand.New(rand.NewSource(117)),
		}, // very good
	}
}

func assignPathQualities_v2() map[string]*PathQuality {
	return map[string]*PathQuality{
		// --- Very good paths (1–7): low RTT and jitter ---
		"d169d3fdcf2a7b89091fb85c61222c3cb3fed169bfa3994a49891042917553e7": {
			rttRange: []float64{10, 20},
			meanRTT:  15, jitter: 1.0, rng: rand.New(rand.NewSource(100)),
		}, // very good

		"5fb945f4d62fb9a9877489f10006d8f9c5a94f16656121e5eb78a0394a4f466e": {
			rttRange: []float64{10, 20},
			meanRTT:  14, jitter: 1.0, rng: rand.New(rand.NewSource(101)),
		}, // very good

		"6c4f86d4c33b5c2371494582f3224b1a3c0611709168c4f411f66c9fa27d744b": {
			rttRange: []float64{12, 22},
			meanRTT:  16, jitter: 1.5, rng: rand.New(rand.NewSource(102)),
		}, // very good

		"e1bf6b79f7219745133909e2a6fba731b92d001007cf81235cec08f3491bf196": {
			rttRange: []float64{15, 25},
			meanRTT:  18, jitter: 1.2, rng: rand.New(rand.NewSource(103)),
		}, // very good

		"b1319d5e0f1cc40a0d227ef38e03ee6d58bd352393cb48f6fbeea0d29032b036": {
			rttRange: []float64{10, 20},
			meanRTT:  15, jitter: 0.8, rng: rand.New(rand.NewSource(104)),
		}, // very good

		"a7a0a2901cccd754534d65374136b9829a7f81518309af79d87f0a560a14690c": {
			rttRange: []float64{10, 20},
			meanRTT:  13, jitter: 1.0, rng: rand.New(rand.NewSource(105)),
		}, // very good

		"02bf95b27afd43858f75306b7a77c5d548ab4618a12cb155775ead71345028f9": {
			rttRange: []float64{10, 20},
			meanRTT:  14, jitter: 1.2, rng: rand.New(rand.NewSource(106)),
		}, // very good

		// --- Ranging from very good to bad ---
		"1c91e925e44aebc06938ee1f41ad22d0c2c3691a877eaf57eb974088bcbd2e4e": {
			rttRange: []float64{20, 30},
			meanRTT:  25, jitter: 1.5, rng: rand.New(rand.NewSource(107)),
		}, // very good

		"4194af709787d03483b2c8f298a1e12fba6abb384a62ded807cb4cb7ee197fd7": {
			rttRange: []float64{40, 80},
			meanRTT:  60, jitter: 6, rng: rand.New(rand.NewSource(108)),
		}, // mid

		"877b3a86f6e88d9034423fde74939b9362cbc8ac368cec0be7eab8e5a9663c6e": {
			rttRange: []float64{50, 60},
			meanRTT:  55, jitter: 3, rng: rand.New(rand.NewSource(109)),
		}, // mid

		"108acbc64efbf5a04652c15db371381c37fba6646fcc000bde7b7877cc8db557": {
			rttRange: []float64{10, 20},
			meanRTT:  15, jitter: 1, rng: rand.New(rand.NewSource(110)),
		}, // very good

		"3496ccc115ec697f3e2027c1f2b70a364bc08cf49ecb6fcb335c3989fca26b02": {
			rttRange: []float64{100, 150},
			meanRTT:  130, jitter: 12, rng: rand.New(rand.NewSource(111)),
		}, // very bad

		"1c1badde515e0cba50c1cbddeb792683884d22bcdfde8a6f4722d3be4dc9cb01": {
			rttRange: []float64{80, 90},
			meanRTT:  85, jitter: 5, rng: rand.New(rand.NewSource(112)),
		}, // bad

		"e488d5b91a7b360294f19bad4ddf644c41d7e47c82cb68256bbedc3eed9a6447": {
			rttRange: []float64{30, 40},
			meanRTT:  35, jitter: 1.5, rng: rand.New(rand.NewSource(113)),
		}, // good

		"aa170b5d350ffdd234572466a5b9e557c6930d2d8669a3f12a813c9410a0c0c7": {
			rttRange: []float64{20, 50},
			meanRTT:  40, jitter: 4, rng: rand.New(rand.NewSource(114)),
		}, // mid

		"c138518cd3637650efb49977186c920375cc6b36f75923be55d04f8f9303e188": {
			rttRange: []float64{10, 100},
			meanRTT:  70, jitter: 15, rng: rand.New(rand.NewSource(115)),
		}, // bad

		"0e8ebf758be09f95eb8f9f5df6eadad00cece2207a4ced88eac43dfafc68597f": {
			rttRange: []float64{22, 39},
			meanRTT:  30, jitter: 2, rng: rand.New(rand.NewSource(116)),
		}, // good

		"d8c9132fa9db80172bcd51547b87137986ee0533e49483ce4fcba9c277c26e4a": {
			rttRange: []float64{10, 15},
			meanRTT:  13, jitter: 0.8, rng: rand.New(rand.NewSource(117)),
		}, // very good
	}
}
