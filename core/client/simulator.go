package client

import (
	"context"
	"fmt"
	"log/slog"
	"math"
	"math/rand"
	"strconv"
	"strings"
	"time"

	"example.com/scion-time/base/logbase"
	"example.com/scion-time/driver/shm"
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
	worsenPaths   []string
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

	pqs := assignPathQualities_exp2()

	worsenPaths := []string{"877b3a86f6e88d9034423fde74939b9362cbc8ac368cec0be7eab8e5a9663c6e", "3496ccc115ec697f3e2027c1f2b70a364bc08cf49ecb6fcb335c3989fca26b02", "1c1badde515e0cba50c1cbddeb792683884d22bcdfde8a6f4722d3be4dc9cb01", "1c91e925e44aebc06938ee1f41ad22d0c2c3691a877eaf57eb974088bcbd2e4e"}

	return &Simulator{log: log, SHM: refClock[0], pathQualities: pqs, worsenPaths: worsenPaths}
}

func (s Simulator) generateTimeStamps(ctx context.Context, p snet.Path, msg string, worsen bool) TimeStamps {
	t3, off := s.t3, s.off // fetched by shmRefClock
	// t3, off := time.Now(), time.Duration(0)

	if t3.IsZero() || (t3.Hour() == 0 && t3.Minute() == 0 && t3.Second() == 0) {
		panic(fmt.Sprintf("PANIC: Fetched time from SHM and t3 is zero."))
	}

	// t3, off := time.Now(), time.Duration(0)

	// USE pathQualities MAP TO DERIVE RTT AND ASYM
	pq := s.pathQualities[snet.Fingerprint(p).String()]

	if worsen { // worsen if second dyn sel running 2. time
		fp := snet.Fingerprint(p).String()
		ok := false
		for _, s := range s.worsenPaths {
			if s == fp {
				ok = true
			}
		}
		if ok { // path is in worsenpaths
			pq.meanRTT = 100
			pq.jitter = 1.6
			pq.rttRange[0] = 50
			pq.rttRange[1] = 200
			fmt.Println("Worsen path:", fp)
		}
	}

	logMsgPQ := fmt.Sprintf(
		"PathQuality | rttRange: [%.2f, %.2f] ms | meanRTT: %.2f ms | jitter: %.2f ms | RNG seed: %d",
		//snet.Fingerprint(p).String(),
		pq.rttRange[0],
		pq.rttRange[1],
		pq.meanRTT,
		pq.jitter,
		pq.rng.Int63(), // get a value from rng to show it's seeded
	)
	// fmt.Println(logMsgPQ)

	// Step 2: Sample RTT
	sampledRTT := float64(sampleClampedNormal(
		pq.rng,
		pq.meanRTT,
		pq.jitter,
		pq.rttRange[0],
		pq.rttRange[1],
	))
	// Step 3: Sample asymmetry

	// Link asymmetry variance to path jitter
	// e.g. 1.0 jitter -> stddev of 10% RTT, 4.0 jitter -> stddev of 40%
	asymStdDev := 0.005 * pq.jitter * sampledRTT
	asymNs := pq.rng.NormFloat64() * asymStdDev

	// delays d0 and d1 can be 75% & 25% or 25% & 75% of rtt at most
	maxAsym := 0.5 * sampledRTT
	if asymNs < -maxAsym {
		asymNs = -maxAsym
	} else if asymNs > maxAsym {
		asymNs = maxAsym
	}
	asym := time.Duration(asymNs)

	// asym = time.Duration(0) // NO ASYMMETRY

	// Step 4: Compute one-way delays (true delays, gnss time perspective)
	d1 := time.Duration(sampledRTT)/2 - asym/2 // client <- server
	d0 := time.Duration(sampledRTT)/2 + asym/2 // client -> server
	d1_float := sampledRTT/2.0 - asymNs/2.0
	d0_float := sampledRTT/2.0 + asymNs/2.0

	// Step 5: Reconstruct the rest
	t2 := t3.Add(-d1).Add(off)  // GNSS time (server send)
	t1 := t2                    // no processing delay at server
	t0 := t1.Add(-d0).Add(-off) // local clock (client send)

	ts := TimeStamps{
		t0: t0, // client send (local)
		t1: t1, // server recv (GNSS)
		t2: t2, // server send (GNSS)
		t3: t3, // client recv (local)
	}

	d0_sim := t1.Sub(t0).Seconds()
	d1_sim := t3.Sub(t2).Seconds()
	asym_sim := math.Abs(d0_sim - d1_sim)
	logMsg := fmt.Sprintf(
		"Reason: %s | FP: %s | Generated timestamps: t0=%s, t1=%s, t2=%s, t3=%s | Real delay asym: %v [micros] | Client delay asym: %v [micros] | Real delays: d0=%v, d1=%v | Client delays: d0=%v, d1=%v [ms] | d0+d1?=rtt: %t | %s",
		msg,
		snet.Fingerprint(p).String(),
		ts.t0.Format("15:04:05.000000"),
		ts.t1.Format("15:04:05.000000"),
		ts.t2.Format("15:04:05.000000"),
		ts.t3.Format("15:04:05.000000"),
		math.Abs(d1.Seconds()-d0.Seconds())*1e6,
		asym_sim*1e6,
		d0,
		d1,
		d0_sim*1000,
		d1_sim*1000,
		sampledRTT == d0_float+d1_float,
		logMsgPQ,
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
			rttRange: []float64{16, 20},
			meanRTT:  15, jitter: 1.0, rng: rand.New(rand.NewSource(100)),
		}, // very good

		"5fb945f4d62fb9a9877489f10006d8f9c5a94f16656121e5eb78a0394a4f466e": {
			rttRange: []float64{16, 20},
			meanRTT:  14, jitter: 1.0, rng: rand.New(rand.NewSource(101)),
		}, // very good

		"6c4f86d4c33b5c2371494582f3224b1a3c0611709168c4f411f66c9fa27d744b": {
			rttRange: []float64{16, 22},
			meanRTT:  16, jitter: 1.5, rng: rand.New(rand.NewSource(102)),
		}, // very good

		"e1bf6b79f7219745133909e2a6fba731b92d001007cf81235cec08f3491bf196": {
			rttRange: []float64{18, 25},
			meanRTT:  18, jitter: 1.2, rng: rand.New(rand.NewSource(103)),
		}, // very good

		"b1319d5e0f1cc40a0d227ef38e03ee6d58bd352393cb48f6fbeea0d29032b036": {
			rttRange: []float64{17, 20},
			meanRTT:  15, jitter: 0.8, rng: rand.New(rand.NewSource(104)),
		}, // very good

		"a7a0a2901cccd754534d65374136b9829a7f81518309af79d87f0a560a14690c": {
			rttRange: []float64{17, 20},
			meanRTT:  13, jitter: 1.0, rng: rand.New(rand.NewSource(105)),
		}, // very good

		"02bf95b27afd43858f75306b7a77c5d548ab4618a12cb155775ead71345028f9": {
			rttRange: []float64{17, 20},
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

// ------------------v4 good init, v5 bad init----------------------------
func assignPathQualities_v4() map[string]*PathQuality {
	return map[string]*PathQuality{
		// --- Very good paths (1–7): low RTT and jitter ---
		"d169d3fdcf2a7b89091fb85c61222c3cb3fed169bfa3994a49891042917553e7": {
			rttRange: []float64{23, 47}, // bounds initial rtt gaussian selection, reject large outliers
			meanRTT:  24, jitter: 0.2, rng: rand.New(rand.NewSource(101)),
			// 99.7% of values from a normal distribution lie within ±3σ (i.e. [23.4, 24.6] in this case)
			// Based on the table at the bottom, we will have asymmetry of 20 microseconds
		},

		"5fb945f4d62fb9a9877489f10006d8f9c5a94f16656121e5eb78a0394a4f466e": {
			rttRange: []float64{23, 47},
			meanRTT:  24, jitter: 0.25, rng: rand.New(rand.NewSource(102)),
		},

		"6c4f86d4c33b5c2371494582f3224b1a3c0611709168c4f411f66c9fa27d744b": {
			rttRange: []float64{23, 47},
			meanRTT:  26, jitter: 0.2, rng: rand.New(rand.NewSource(103)),
		},

		"e1bf6b79f7219745133909e2a6fba731b92d001007cf81235cec08f3491bf196": {
			rttRange: []float64{23, 47},
			meanRTT:  25, jitter: 0.1, rng: rand.New(rand.NewSource(104)),
		},

		"b1319d5e0f1cc40a0d227ef38e03ee6d58bd352393cb48f6fbeea0d29032b036": {
			rttRange: []float64{23, 47},
			meanRTT:  31.7, jitter: 0.19, rng: rand.New(rand.NewSource(105)),
		},

		"a7a0a2901cccd754534d65374136b9829a7f81518309af79d87f0a560a14690c": {
			rttRange: []float64{23, 47},
			meanRTT:  33, jitter: 0.2, rng: rand.New(rand.NewSource(106)),
		},

		"02bf95b27afd43858f75306b7a77c5d548ab4618a12cb155775ead71345028f9": {
			rttRange: []float64{23, 47},
			meanRTT:  28, jitter: 0.22, rng: rand.New(rand.NewSource(107)),
		},

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

func assignPathQualities_v5() map[string]*PathQuality {
	return map[string]*PathQuality{
		// --- Very good paths (1–7): low RTT and jitter ---
		"d169d3fdcf2a7b89091fb85c61222c3cb3fed169bfa3994a49891042917553e7": {
			rttRange: []float64{23, 77}, // bounds initial rtt gaussian selection, reject large outliers
			meanRTT:  46, jitter: 0.3, rng: rand.New(rand.NewSource(101)),
			// 99.7% of values from a normal distribution lie within ±3σ (i.e. [23.4, 24.6] in this case)
			// Based on the table at the bottom, we will have asymmetry of 20 microseconds
		},

		"5fb945f4d62fb9a9877489f10006d8f9c5a94f16656121e5eb78a0394a4f466e": {
			rttRange: []float64{23, 77},
			meanRTT:  50, jitter: 0.25, rng: rand.New(rand.NewSource(102)),
		},

		"6c4f86d4c33b5c2371494582f3224b1a3c0611709168c4f411f66c9fa27d744b": {
			rttRange: []float64{23, 77},
			meanRTT:  46, jitter: 0.24, rng: rand.New(rand.NewSource(103)),
		},

		"e1bf6b79f7219745133909e2a6fba731b92d001007cf81235cec08f3491bf196": {
			rttRange: []float64{23, 77},
			meanRTT:  38.2, jitter: 0.4, rng: rand.New(rand.NewSource(104)),
		},

		"b1319d5e0f1cc40a0d227ef38e03ee6d58bd352393cb48f6fbeea0d29032b036": {
			rttRange: []float64{23, 77},
			meanRTT:  45.2, jitter: 0.3, rng: rand.New(rand.NewSource(105)),
		},

		"a7a0a2901cccd754534d65374136b9829a7f81518309af79d87f0a560a14690c": {
			rttRange: []float64{23, 77},
			meanRTT:  39, jitter: 0.32, rng: rand.New(rand.NewSource(106)),
		},

		"02bf95b27afd43858f75306b7a77c5d548ab4618a12cb155775ead71345028f9": {
			rttRange: []float64{23, 77},
			meanRTT:  42, jitter: 0.26, rng: rand.New(rand.NewSource(107)),
		},

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

// very bad init paths
func assignPathQualities_v6() map[string]*PathQuality {
	return map[string]*PathQuality{
		// --- Very good paths (1–7): low RTT and jitter ---
		"d169d3fdcf2a7b89091fb85c61222c3cb3fed169bfa3994a49891042917553e7": {
			rttRange: []float64{50, 60},
			meanRTT:  55, jitter: 0.8, rng: rand.New(rand.NewSource(101)),
		},

		"5fb945f4d62fb9a9877489f10006d8f9c5a94f16656121e5eb78a0394a4f466e": {
			rttRange: []float64{30, 45},
			meanRTT:  38, jitter: 0.6, rng: rand.New(rand.NewSource(102)),
		},

		"6c4f86d4c33b5c2371494582f3224b1a3c0611709168c4f411f66c9fa27d744b": {
			rttRange: []float64{40, 60},
			meanRTT:  50, jitter: 0.7, rng: rand.New(rand.NewSource(103)),
		},

		"e1bf6b79f7219745133909e2a6fba731b92d001007cf81235cec08f3491bf196": {
			rttRange: []float64{60, 85},
			meanRTT:  70, jitter: 1.0, rng: rand.New(rand.NewSource(104)),
		},

		"b1319d5e0f1cc40a0d227ef38e03ee6d58bd352393cb48f6fbeea0d29032b036": {
			rttRange: []float64{85, 95},
			meanRTT:  90, jitter: 1.2, rng: rand.New(rand.NewSource(105)),
		},

		"a7a0a2901cccd754534d65374136b9829a7f81518309af79d87f0a560a14690c": {
			rttRange: []float64{80, 120},
			meanRTT:  100, jitter: 1.5, rng: rand.New(rand.NewSource(106)),
		},

		"02bf95b27afd43858f75306b7a77c5d548ab4618a12cb155775ead71345028f9": {
			rttRange: []float64{120, 160},
			meanRTT:  140, jitter: 1.8, rng: rand.New(rand.NewSource(107)),
		},

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

func assignPathQualities_exp4() map[string]*PathQuality {
	return map[string]*PathQuality{
		// --- Very good paths (1–7): low RTT and jitter ---
		"d169d3fdcf2a7b89091fb85c61222c3cb3fed169bfa3994a49891042917553e7": {
			rttRange: []float64{50, 60},
			meanRTT:  55, jitter: 0.8, rng: rand.New(rand.NewSource(101)),
		},

		"5fb945f4d62fb9a9877489f10006d8f9c5a94f16656121e5eb78a0394a4f466e": {
			rttRange: []float64{30, 45},
			meanRTT:  38, jitter: 0.6, rng: rand.New(rand.NewSource(102)),
		},

		"6c4f86d4c33b5c2371494582f3224b1a3c0611709168c4f411f66c9fa27d744b": {
			rttRange: []float64{40, 60},
			meanRTT:  50, jitter: 0.7, rng: rand.New(rand.NewSource(103)),
		},

		"e1bf6b79f7219745133909e2a6fba731b92d001007cf81235cec08f3491bf196": {
			rttRange: []float64{60, 85},
			meanRTT:  70, jitter: 1.0, rng: rand.New(rand.NewSource(104)),
		},

		"b1319d5e0f1cc40a0d227ef38e03ee6d58bd352393cb48f6fbeea0d29032b036": {
			rttRange: []float64{85, 95},
			meanRTT:  90, jitter: 1.2, rng: rand.New(rand.NewSource(105)),
		},

		"a7a0a2901cccd754534d65374136b9829a7f81518309af79d87f0a560a14690c": {
			rttRange: []float64{80, 120},
			meanRTT:  100, jitter: 1.5, rng: rand.New(rand.NewSource(106)),
		},

		"02bf95b27afd43858f75306b7a77c5d548ab4618a12cb155775ead71345028f9": {
			rttRange: []float64{120, 160},
			meanRTT:  140, jitter: 1.8, rng: rand.New(rand.NewSource(107)),
		},

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

func assignPathQualities_exp1() map[string]*PathQuality {
	return map[string]*PathQuality{
		// --- Very bad paths (1–7) ---
		"d169d3fdcf2a7b89091fb85c61222c3cb3fed169bfa3994a49891042917553e7": {
			rttRange: []float64{50, 60},
			meanRTT:  55, jitter: 0.8, rng: rand.New(rand.NewSource(101)),
		},

		"5fb945f4d62fb9a9877489f10006d8f9c5a94f16656121e5eb78a0394a4f466e": {
			rttRange: []float64{30, 45},
			meanRTT:  38, jitter: 0.6, rng: rand.New(rand.NewSource(102)),
		},

		"6c4f86d4c33b5c2371494582f3224b1a3c0611709168c4f411f66c9fa27d744b": {
			rttRange: []float64{40, 60},
			meanRTT:  50, jitter: 0.7, rng: rand.New(rand.NewSource(103)),
		},

		"e1bf6b79f7219745133909e2a6fba731b92d001007cf81235cec08f3491bf196": {
			rttRange: []float64{60, 85},
			meanRTT:  70, jitter: 1.0, rng: rand.New(rand.NewSource(104)),
		},

		"b1319d5e0f1cc40a0d227ef38e03ee6d58bd352393cb48f6fbeea0d29032b036": {
			rttRange: []float64{85, 95},
			meanRTT:  90, jitter: 1.2, rng: rand.New(rand.NewSource(105)),
		},

		"a7a0a2901cccd754534d65374136b9829a7f81518309af79d87f0a560a14690c": {
			rttRange: []float64{80, 120},
			meanRTT:  100, jitter: 1.5, rng: rand.New(rand.NewSource(106)),
		},

		"02bf95b27afd43858f75306b7a77c5d548ab4618a12cb155775ead71345028f9": {
			rttRange: []float64{120, 160},
			meanRTT:  140, jitter: 1.8, rng: rand.New(rand.NewSource(107)),
		},

		// --- All the same so that we know they have same properties but it depends on the moment (seed)---
		"1c91e925e44aebc06938ee1f41ad22d0c2c3691a877eaf57eb974088bcbd2e4e": {
			rttRange: []float64{20, 50},
			meanRTT:  30, jitter: 0.6, rng: rand.New(rand.NewSource(118)),
		},

		"4194af709787d03483b2c8f298a1e12fba6abb384a62ded807cb4cb7ee197fd7": {
			rttRange: []float64{20, 30},
			meanRTT:  25, jitter: 0.8, rng: rand.New(rand.NewSource(108)),
		},

		"877b3a86f6e88d9034423fde74939b9362cbc8ac368cec0be7eab8e5a9663c6e": {
			rttRange: []float64{20, 50},
			meanRTT:  30, jitter: 0.6, rng: rand.New(rand.NewSource(109)),
		},

		"108acbc64efbf5a04652c15db371381c37fba6646fcc000bde7b7877cc8db557": {
			rttRange: []float64{20, 30},
			meanRTT:  25, jitter: 0.8, rng: rand.New(rand.NewSource(110)),
		},

		"3496ccc115ec697f3e2027c1f2b70a364bc08cf49ecb6fcb335c3989fca26b02": {
			rttRange: []float64{20, 50},
			meanRTT:  30, jitter: 0.6, rng: rand.New(rand.NewSource(111)),
		},

		"1c1badde515e0cba50c1cbddeb792683884d22bcdfde8a6f4722d3be4dc9cb01": {
			rttRange: []float64{20, 30},
			meanRTT:  25, jitter: 0.8, rng: rand.New(rand.NewSource(112)),
		},

		"e488d5b91a7b360294f19bad4ddf644c41d7e47c82cb68256bbedc3eed9a6447": {
			rttRange: []float64{20, 50},
			meanRTT:  30, jitter: 0.6, rng: rand.New(rand.NewSource(113)),
		},

		"aa170b5d350ffdd234572466a5b9e557c6930d2d8669a3f12a813c9410a0c0c7": {
			rttRange: []float64{20, 30},
			meanRTT:  25, jitter: 0.8, rng: rand.New(rand.NewSource(114)),
		},

		"c138518cd3637650efb49977186c920375cc6b36f75923be55d04f8f9303e188": {
			rttRange: []float64{20, 50},
			meanRTT:  30, jitter: 0.6, rng: rand.New(rand.NewSource(115)),
		},

		"0e8ebf758be09f95eb8f9f5df6eadad00cece2207a4ced88eac43dfafc68597f": {
			rttRange: []float64{20, 30},
			meanRTT:  25, jitter: 0.8, rng: rand.New(rand.NewSource(116)),
		},

		"d8c9132fa9db80172bcd51547b87137986ee0533e49483ce4fcba9c277c26e4a": {
			rttRange: []float64{20, 50},
			meanRTT:  30, jitter: 0.6, rng: rand.New(rand.NewSource(117)),
		},
	}
}

func assignPathQualities_exp3() map[string]*PathQuality {
	return map[string]*PathQuality{
		// --- Very bad paths (1–7) ---------------------------------------------------------------------------------------------------------------------------------------------------------
		"d169d3fdcf2a7b89091fb85c61222c3cb3fed169bfa3994a49891042917553e7": {
			rttRange: []float64{50, 60},
			meanRTT:  55, jitter: 0.8, rng: rand.New(rand.NewSource(101)),
		},

		"5fb945f4d62fb9a9877489f10006d8f9c5a94f16656121e5eb78a0394a4f466e": {
			rttRange: []float64{30, 45},
			meanRTT:  40, jitter: 1, rng: rand.New(rand.NewSource(102)),
		},

		"6c4f86d4c33b5c2371494582f3224b1a3c0611709168c4f411f66c9fa27d744b": {
			rttRange: []float64{40, 60},
			meanRTT:  50, jitter: 0.7, rng: rand.New(rand.NewSource(103)),
		},

		"e1bf6b79f7219745133909e2a6fba731b92d001007cf81235cec08f3491bf196": {
			rttRange: []float64{60, 85},
			meanRTT:  70, jitter: 1.0, rng: rand.New(rand.NewSource(104)),
		},

		"b1319d5e0f1cc40a0d227ef38e03ee6d58bd352393cb48f6fbeea0d29032b036": {
			rttRange: []float64{85, 95},
			meanRTT:  90, jitter: 1.2, rng: rand.New(rand.NewSource(105)),
		},

		"a7a0a2901cccd754534d65374136b9829a7f81518309af79d87f0a560a14690c": {
			rttRange: []float64{80, 120},
			meanRTT:  100, jitter: 1.5, rng: rand.New(rand.NewSource(106)),
		},

		"02bf95b27afd43858f75306b7a77c5d548ab4618a12cb155775ead71345028f9": {
			rttRange: []float64{120, 160},
			meanRTT:  140, jitter: 1.8, rng: rand.New(rand.NewSource(107)),
		},

		// --- 7 very good---------------------------------------------------------------------------------------------------------------------------------------------------------
		"1c91e925e44aebc06938ee1f41ad22d0c2c3691a877eaf57eb974088bcbd2e4e": {
			rttRange: []float64{20, 50},
			meanRTT:  30, jitter: 0.6, rng: rand.New(rand.NewSource(118)),
		},

		"4194af709787d03483b2c8f298a1e12fba6abb384a62ded807cb4cb7ee197fd7": {
			rttRange: []float64{20, 30},
			meanRTT:  25, jitter: 0.8, rng: rand.New(rand.NewSource(108)),
		},

		"877b3a86f6e88d9034423fde74939b9362cbc8ac368cec0be7eab8e5a9663c6e": {
			rttRange: []float64{20, 50},
			meanRTT:  30, jitter: 0.6, rng: rand.New(rand.NewSource(109)),
		},

		"108acbc64efbf5a04652c15db371381c37fba6646fcc000bde7b7877cc8db557": {
			rttRange: []float64{20, 30},
			meanRTT:  25, jitter: 0.8, rng: rand.New(rand.NewSource(110)),
		},

		"3496ccc115ec697f3e2027c1f2b70a364bc08cf49ecb6fcb335c3989fca26b02": {
			rttRange: []float64{20, 50},
			meanRTT:  30, jitter: 0.6, rng: rand.New(rand.NewSource(111)),
		},

		"1c1badde515e0cba50c1cbddeb792683884d22bcdfde8a6f4722d3be4dc9cb01": {
			rttRange: []float64{20, 30},
			meanRTT:  25, jitter: 0.8, rng: rand.New(rand.NewSource(112)),
		},

		"e488d5b91a7b360294f19bad4ddf644c41d7e47c82cb68256bbedc3eed9a6447": {
			rttRange: []float64{20, 50},
			meanRTT:  30, jitter: 0.6, rng: rand.New(rand.NewSource(113)),
		},
		// --- 4 good, bit worse than 7 very good---------------------------------------------------------------------------
		"aa170b5d350ffdd234572466a5b9e557c6930d2d8669a3f12a813c9410a0c0c7": {
			rttRange: []float64{20, 50},
			meanRTT:  34, jitter: 0.8, rng: rand.New(rand.NewSource(114)),
		},

		"c138518cd3637650efb49977186c920375cc6b36f75923be55d04f8f9303e188": {
			rttRange: []float64{20, 50},
			meanRTT:  34, jitter: 0.8, rng: rand.New(rand.NewSource(115)),
		},

		"0e8ebf758be09f95eb8f9f5df6eadad00cece2207a4ced88eac43dfafc68597f": {
			rttRange: []float64{20, 50},
			meanRTT:  34, jitter: 0.8, rng: rand.New(rand.NewSource(116)),
		},

		"d8c9132fa9db80172bcd51547b87137986ee0533e49483ce4fcba9c277c26e4a": {
			rttRange: []float64{20, 50},
			meanRTT:  34, jitter: 0.8, rng: rand.New(rand.NewSource(117)),
		},
	}
}

func assignPathQualities_exp2() map[string]*PathQuality {
	return map[string]*PathQuality{
		// --- Very bad paths (1–7) ---------------------------------------------------------------------------------------------------------------------------------------------------------
		"d169d3fdcf2a7b89091fb85c61222c3cb3fed169bfa3994a49891042917553e7": {
			rttRange: []float64{50, 60},
			meanRTT:  55, jitter: 0.8, rng: rand.New(rand.NewSource(101)),
		},

		"5fb945f4d62fb9a9877489f10006d8f9c5a94f16656121e5eb78a0394a4f466e": {
			rttRange: []float64{30, 45},
			meanRTT:  40, jitter: 1, rng: rand.New(rand.NewSource(102)),
		},

		"6c4f86d4c33b5c2371494582f3224b1a3c0611709168c4f411f66c9fa27d744b": {
			rttRange: []float64{40, 60},
			meanRTT:  50, jitter: 0.7, rng: rand.New(rand.NewSource(103)),
		},

		"e1bf6b79f7219745133909e2a6fba731b92d001007cf81235cec08f3491bf196": {
			rttRange: []float64{60, 85},
			meanRTT:  70, jitter: 1.0, rng: rand.New(rand.NewSource(104)),
		},

		"b1319d5e0f1cc40a0d227ef38e03ee6d58bd352393cb48f6fbeea0d29032b036": {
			rttRange: []float64{85, 95},
			meanRTT:  90, jitter: 1.2, rng: rand.New(rand.NewSource(105)),
		},

		"a7a0a2901cccd754534d65374136b9829a7f81518309af79d87f0a560a14690c": {
			rttRange: []float64{80, 120},
			meanRTT:  100, jitter: 1.5, rng: rand.New(rand.NewSource(106)),
		},

		"02bf95b27afd43858f75306b7a77c5d548ab4618a12cb155775ead71345028f9": {
			rttRange: []float64{120, 160},
			meanRTT:  140, jitter: 1.8, rng: rand.New(rand.NewSource(107)),
		},

		// --- 7 very good---------------------------------------------------------------------------------------------------------------------------------------------------------
		"1c91e925e44aebc06938ee1f41ad22d0c2c3691a877eaf57eb974088bcbd2e4e": {
			rttRange: []float64{20, 50},
			meanRTT:  30, jitter: 0.6, rng: rand.New(rand.NewSource(118)),
		},

		"4194af709787d03483b2c8f298a1e12fba6abb384a62ded807cb4cb7ee197fd7": {
			rttRange: []float64{20, 30},
			meanRTT:  25, jitter: 0.8, rng: rand.New(rand.NewSource(108)),
		},

		"877b3a86f6e88d9034423fde74939b9362cbc8ac368cec0be7eab8e5a9663c6e": {
			rttRange: []float64{20, 50},
			meanRTT:  30, jitter: 0.6, rng: rand.New(rand.NewSource(109)),
		},

		"108acbc64efbf5a04652c15db371381c37fba6646fcc000bde7b7877cc8db557": {
			rttRange: []float64{20, 30},
			meanRTT:  25, jitter: 0.8, rng: rand.New(rand.NewSource(110)),
		},

		"3496ccc115ec697f3e2027c1f2b70a364bc08cf49ecb6fcb335c3989fca26b02": {
			rttRange: []float64{20, 50},
			meanRTT:  30, jitter: 0.6, rng: rand.New(rand.NewSource(111)),
		},

		"1c1badde515e0cba50c1cbddeb792683884d22bcdfde8a6f4722d3be4dc9cb01": {
			rttRange: []float64{20, 30},
			meanRTT:  25, jitter: 0.8, rng: rand.New(rand.NewSource(112)),
		},

		"e488d5b91a7b360294f19bad4ddf644c41d7e47c82cb68256bbedc3eed9a6447": {
			rttRange: []float64{20, 50},
			meanRTT:  30, jitter: 0.6, rng: rand.New(rand.NewSource(113)),
		},
		// --- 4 good, bit worse than 7 very good---------------------------------------------------------------------------
		"aa170b5d350ffdd234572466a5b9e557c6930d2d8669a3f12a813c9410a0c0c7": {
			rttRange: []float64{20, 50},
			meanRTT:  34, jitter: 0.8, rng: rand.New(rand.NewSource(114)),
		},

		"c138518cd3637650efb49977186c920375cc6b36f75923be55d04f8f9303e188": {
			rttRange: []float64{20, 50},
			meanRTT:  34, jitter: 0.8, rng: rand.New(rand.NewSource(115)),
		},

		"0e8ebf758be09f95eb8f9f5df6eadad00cece2207a4ced88eac43dfafc68597f": {
			rttRange: []float64{20, 50},
			meanRTT:  34, jitter: 0.8, rng: rand.New(rand.NewSource(116)),
		},

		"d8c9132fa9db80172bcd51547b87137986ee0533e49483ce4fcba9c277c26e4a": {
			rttRange: []float64{20, 50},
			meanRTT:  34, jitter: 0.8, rng: rand.New(rand.NewSource(117)),
		},
	}
}

/*
Guideline for setting path specs
rtt=20ms
| Jitter (`pq.jitter`) |     Asym Std Dev (`asymStdDev` in ns) | Asym Std Dev (µs) |
| -------------------: | ------------------------------------: | ----------------: |
|                  0.2 | 0.005 × 0.2 × 20,000,000 = **20,000** |         **20 µs** |
|                  0.4 |                                40,000 |             40 µs |
|                  0.5 |                                50,000 |             50 µs |
|                  0.6 |                                60,000 |             60 µs |
|                  0.8 |                                80,000 |             80 µs |
|                  1.0 |                               100,000 |            100 µs |
|                  1.2 |                               120,000 |            120 µs |
|                  1.4 |                               140,000 |            140 µs |
|                  1.5 |                               150,000 |            150 µs |
|                  1.6 |                               160,000 |            160 µs |

rtt=25ms
Jitter (pq.jitter)	Asym Std Dev (asymStdDev in ns)	Asym Std Dev (µs)
0.2	0.005 × 0.2 × 25,000,000 = 25,000	25 µs
0.4	50,000	50 µs
0.5	62,500	62 µs
0.6	75,000	75 µs
0.8	100,000	100 µs
1.0	125,000	125 µs
1.2	150,000	150 µs
1.4	175,000	175 µs
1.5	187,500	187 µs
1.6	200,000	200 µs

rtt=30ms
| Jitter (`pq.jitter`) |       Asym Std Dev (`asymStdDev` in ns) | Asym Std Dev (µs) |
| -------------------: | --------------------------------------: | ----------------: |
|                  0.2 | 0.005 × 0.2 × 30\_000\_000 = **30,000** |         **30 µs** |
|                  0.4 |                                  60,000 |             60 µs |
|                  0.5 |                                  75,000 |             75 µs |
|                  0.6 |                                  90,000 |             90 µs |
|                  0.8 |                                 120,000 |            120 µs |
|                  1.0 |                                 150,000 |            150 µs |
|                  1.2 |                                 180,000 |            180 µs |
|                  1.4 |                                 210,000 |            210 µs |
|                  1.5 |                                 225,000 |            225 µs |
|                  1.6 |                                 240,000 |            240 µs |

rtt=35ms
Jitter (pq.jitter)	Asym Std Dev (asymStdDev in ns)	Asym Std Dev (µs)
0.2	0.005 × 0.2 × 35,000,000 = 35,000	35 µs
0.4	70,000	70 µs
0.5	87,500	87 µs
0.6	105,000	105 µs
0.8	140,000	140 µs
1.0	175,000	175 µs
1.2	210,000	210 µs
1.4	245,000	245 µs
1.5	262,500	262 µs
1.6	280,000	280 µs

rtt=40ms
| Jitter (`pq.jitter`) |     Asym Std Dev (`asymStdDev` in ns) | Asym Std Dev (µs) |
| -------------------: | ------------------------------------: | ----------------: |
|                  0.2 | 0.005 × 0.2 × 40,000,000 = **40,000** |         **40 µs** |
|                  0.4 |                                80,000 |             80 µs |
|                  0.5 |                               100,000 |            100 µs |
|                  0.6 |                               120,000 |            120 µs |
|                  0.8 |                               160,000 |            160 µs |
|                  1.0 |                               200,000 |            200 µs |
|                  1.2 |                               240,000 |            240 µs |
|                  1.4 |                               280,000 |            280 µs |
|                  1.5 |                               300,000 |            300 µs |
|                  1.6 |                               320,000 |            320 µs |

rtt=45ms
Jitter (pq.jitter)	Asym Std Dev (asymStdDev in ns)	Asym Std Dev (µs)
0.2	0.005 × 0.2 × 45,000,000 = 45,000	45 µs
0.4	90,000	90 µs
0.5	112,500	112 µs
0.6	135,000	135 µs
0.8	180,000	180 µs
1.0	225,000	225 µs
1.2	270,000	270 µs
1.4	315,000	315 µs
1.5	337,500	337 µs
1.6	360,000	360 µs

rtt=50ms
Jitter (pq.jitter)	Asym Std Dev (asymStdDev in ns)	Asym Std Dev (µs)
0.2	0.005 × 0.2 × 50,000,000 = 50,000	50 µs
0.4	100,000	100 µs
0.5	125,000	125 µs
0.6	150,000	150 µs
0.8	200,000	200 µs
1.0	250,000	250 µs
1.2	300,000	300 µs
1.4	350,000	350 µs
1.5	375,000	375 µs
1.6	400,000	400 µs

rtt=55ms
Jitter (pq.jitter)	Asym Std Dev (asymStdDev in ns)	Asym Std Dev (µs)
0.2	0.005 × 0.2 × 55,000,000 = 55,000	55 µs
0.4	110,000	110 µs
0.5	137,500	137 µs
0.6	165,000	165 µs
0.8	220,000	220 µs
1.0	275,000	275 µs
1.2	330,000	330 µs
1.4	385,000	385 µs
1.5	412,500	412 µs
1.6	440,000	440 µs

rtt=60ms
| Jitter (`pq.jitter`) |       Asym Std Dev (`asymStdDev` in ns) | Asym Std Dev (µs) |
| -------------------: | --------------------------------------: | ----------------: |
|                  0.2 | 0.005 × 0.2 × 60\_000\_000 = **60,000** |         **60 µs** |
|                  0.4 |                                 120,000 |            120 µs |
|                  0.5 |                                 150,000 |            150 µs |
|                  0.6 |                                 180,000 |            180 µs |
|                  0.8 |                                 240,000 |            240 µs |
|                  1.0 |                                 300,000 |            300 µs |
|                  1.2 |                                 360,000 |            360 µs |
|                  1.4 |                                 420,000 |            420 µs |
|                  1.5 |                                 450,000 |            450 µs |
|                  1.6 |                                 480,000 |            480 µs |*/
