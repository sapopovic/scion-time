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
}

type PathQuality struct {
	rttRange []float64  // [min, max] in milliseconds
	meanRTT  float64    // manually defined mean RTT
	jitter   float64    // standard deviation (jitter)
	rng      *rand.Rand // seeded RNG
	asymBias float64
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

	pqs := assignPathQualities_v6()

	return &Simulator{log: log, SHM: refClock[0], pathQualities: pqs}
}

func (s Simulator) generateTimeStamps(ctx context.Context, p snet.Path, msg string) TimeStamps {
	/*
		// Step 1: Fetch t3 and offset from SHM (or use fixed for simulation)
		t3, off := s.t3, s.off // GNSS-based local time and offset
		// Test 2: The only unknown is the drift
		// t3, off := time.Now(), time.Duration(0)

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

		return ts*/
	//---------------------------------------------------------------------------------------------------------------------------------------
	/*t3, off := s.t3, s.off // fetched by shmRefClock

	// USE pathQualities MAP TO DERIVE RTT AND ASYM
	pq := s.pathQualities[snet.Fingerprint(p).String()]

	/////// Step 2: Sample RTT
	minRTT := pq.rttRange[0] // e.g. 10.0 microseconds
	///////maxRTT := pq.rttRange[1] // e.g. 30.0
	///////rttMs := pq.rng.Float64()*(maxRTT-minRTT) + minRTT
	/////// rtt := time.Duration(rttMs * float64(time.Millisecond)) // Duration is in nano seconds, time.Microsecond is 1000 ns = 1 microsecond
	/////
	/////// Step 3: Sample asymmetry
	/////// minAsym := -float64(rtt.Nanoseconds()) // in nanoseconds
	/////// maxAsym := float64(rtt.Nanoseconds())
	/////// asymNs := pq.rng.Float64()*(maxAsym-minAsym-1) + minAsym + 0.5
	/////// asym := time.Duration(asymNs) // asym in nanoseconds

	// asym = time.Duration(0) // NO ASYMMETRY

	// Step 4: Compute one-way delays (true delays, gnss time perspective)
	//////////d1 := rtt/2 - asym/2 // client <- server
	//////////d0 := rtt/2 + asym/2 // client -> server

	rtt := minRTT
	rttNs := rtt
	meanAsym := rttNs / 2
	stddev := 0.05 * rttNs
	asymNs := pq.rng.NormFloat64()*stddev + meanAsym
	// Clamp asym to [0, rtt]
	if asymNs < 0 {
		asymNs = 0
	} else if asymNs > rttNs {
		asymNs = rttNs
	}
	asym := asymNs
	d1 := time.Duration(rtt/2 - asym/2) // client <- server
	d0 := time.Duration(rtt/2 + asym/2) // client -> server

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

	logMsg := fmt.Sprintf(
		"Generated timestamps: t0=%s, t1=%s, t2=%s, t3=%s | delays: d0=%v, d1=%v | SHM offset: off=%v | NTP offset: off=%v",
		ts.t0.Format("15:04:05.000000"),
		ts.t1.Format("15:04:05.000000"),
		ts.t2.Format("15:04:05.000000"),
		ts.t3.Format("15:04:05.000000"),
		d0,
		d1,
		off,
		ntp.ClockOffset(t0, t1, t2, t3),
	)

	fmt.Println(logMsg)

	return ts*/
	//---------------------------------------------------------------------------------------------------------------------------------------
	//THIS
	// Step 1: GNSS-based server send time (t3) and client offset (off)
	/*t3, off := s.t3, s.off

	// Step 2: Path-specific quality settings
	pq := s.pathQualities[snet.Fingerprint(p).String()]

	// Step 3: Sample RTT from clamped normal distribution
	rtt := float64(sampleClampedNormal(
		pq.rng,
		pq.meanRTT,
		pq.jitter,
		pq.rttRange[0],
		pq.rttRange[1],
	)) / float64(time.Millisecond)

	// Step 4: Occasionally inject delay spike (2% of samples)
	if pq.rng.Float64() < 0.02 {
		rtt += 50 + 100*pq.rng.Float64() // +50–150ms spike
	}

	// Step 5: Compute asymmetry (as ns)
	rttNs := rtt * float64(time.Millisecond)
	meanAsym := 0.0                                  // initial asymmetry is 0
	stddev := 0.05 * rttNs                           // asymmetry probability rises with rising rtt
	asymNs := pq.rng.NormFloat64()*stddev + meanAsym // A path with high RTT will allow more variation (and potentially higher NTP error).

	// Step 6: Occasionally inject asymmetry spike (1% of samples)
	chance := pq.rng.Float64()
	//if chance < 0.0015 {
	//	asymNs += 90_000_000 + 20_000_000*pq.rng.Float64()
	//} else if chance < 0.01 {
	//	asymNs += 80_000_000 + 10_000_000*pq.rng.Float64()
	//}
	if chance < 0.0015 {
		spike := 90_000_000 + 20_000_000*pq.rng.Float64()
		if pq.rng.Float64() < 0.5 {
			spike = -spike
		}
		asymNs += spike
	} else if chance < 0.01 {
		spike := 80_000_000 + 10_000_000*pq.rng.Float64()
		if pq.rng.Float64() < 0.5 {
			spike = -spike
		}
		asymNs += spike
	}

	// Clamp asymmetry within valid RTT bounds
	//if asymNs < -rttNs {
	//	asymNs = -rttNs
	//} else if asymNs > rttNs {
	//	asymNs = rttNs
	//}
	minDelay := 100_000.0         // 100 µs in nanoseconds
	maxAsym := rttNs - 2*minDelay // In our simulation, asymmetry cannot be bigger than (rtt-200microseconds). Thus, paths with bigger rtt can have bigger asymmetry.

	if asymNs < -maxAsym { // we cap the asymmetry, can be at most (rtt-200microseconds) difference between d0 and d1
		asymNs = -maxAsym
	} else if asymNs > maxAsym {
		asymNs = maxAsym
	}

	// Step 7: Compute directional delays
	d0 := time.Duration((rttNs + asymNs) / 2)
	d1 := time.Duration((rttNs - asymNs) / 2)

	// Step 8: Add ±0.2ms per-timestamp noise (simulates OS/NIC jitter)
	timestampNoise := func() time.Duration {
		// return time.Duration(pq.rng.NormFloat64() * 200_000) // ±0.2ms=200microseconds
		return time.Duration(0)
	}

	// Step 9: Construct timestamps
	t2 := t3.Add(-d1).Add(off).Add(timestampNoise())  // Server send
	t1 := t2                                          // Server recv = send
	t0 := t1.Add(-d0).Add(-off).Add(timestampNoise()) // Client send

	ts := TimeStamps{
		t0: t0,
		t1: t1,
		t2: t2,
		t3: t3,
	}

	// Step 10: Logging & verification
	logMsg := fmt.Sprintf(
		"via=%s | Generated timestamps: t0=%s, t1=%s, t2=%s, t3=%s | d0=%v, d1=%v | RTT=%v | SHM offset=%v | NTP offset: off=%v",
		snet.Fingerprint(p).String(),
		ts.t0.Format("15:04:05.000000"),
		ts.t1.Format("15:04:05.000000"),
		ts.t2.Format("15:04:05.000000"),
		ts.t3.Format("15:04:05.000000"),
		d0,
		d1,
		d0+d1,
		off,
		ntp.ClockOffset(t0, t1, t2, t3),
	)
	fmt.Println(logMsg)

	return ts*/
	//---------------------------------------------------------------------------------------------------------------------------------------
	/*t3, off := s.t3, s.off // GNSS-based time + client offset
	pq := s.pathQualities[snet.Fingerprint(p).String()]

	baseRTT := pq.rttRange[0]
	d0Base := 0.5 * baseRTT // uplink
	d1Base := 0.5 * baseRTT // downlink

	// Step 1: Sample RTT from clamped normal distribution
	sampledRTT := float64(sampleClampedNormal(
		pq.rng,
		pq.meanRTT,
		pq.jitter,
		pq.rttRange[0],
		pq.rttRange[1],
	))

	// Step 2: Inject occasional RTT spikes (full path spike)
	chance := pq.rng.Float64()
	if chance < 0.001 {
		sampledRTT += 90_000_000 + 20_000_000*pq.rng.Float64()
	} else if chance < 0.01 {
		sampledRTT += 80_000_000 + 10_000_000*pq.rng.Float64()
	}

	delta := sampledRTT - baseRTT

	// Step 3: Asymmetry allocation
	r := pq.asymBias + 0.09*pq.rng.NormFloat64()
	if r < 0.4 {
		r = 0.4
	} else if r > 0.6 {
		r = 0.6
	}

	d0 := d0Base + r*delta
	d1 := d1Base + (1-r)*delta

	// Step 4: Enforce minimum one-way delays
	// const minDelayNs = 500_000.0 // 0.5ms
	// if d0 < minDelayNs {
	// 	d0 = minDelayNs
	// }
	// if d1 < minDelayNs {
	// 	d1 = minDelayNs
	// }

	// Step 5: Optional timestamp jitter (±0.1ms)
	jitterNs := 25_000.0
	d0 += pq.rng.NormFloat64() * jitterNs
	d1 += pq.rng.NormFloat64() * jitterNs

	// Step 6: Build durations and reconstruct timestamps
	d0_duration := time.Duration(d0)
	d1_duration := time.Duration(d1)

	t2 := t3.Add(-d1_duration).Add(off)  // server send
	t1 := t2                             // server receive
	t0 := t1.Add(-d0_duration).Add(-off) // client send

	ts := TimeStamps{
		t0: t0,
		t1: t1,
		t2: t2,
		t3: t3,
	}

	// Step 7: Logging
	logMsg := fmt.Sprintf(
		"via=%s | Generated timestamps: t0=%s, t1=%s, t2=%s, t3=%s | d0=%v, d1=%v | RTT=%v | SHM offset=%v | NTP offset: off=%v",
		snet.Fingerprint(p).String(),
		ts.t0.Format("15:04:05.000000"),
		ts.t1.Format("15:04:05.000000"),
		ts.t2.Format("15:04:05.000000"),
		ts.t3.Format("15:04:05.000000"),
		d0,
		d1,
		d0+d1,
		off,
		ntp.ClockOffset(t0, t1, t2, t3),
	)
	fmt.Println(logMsg)

	return ts*/
	t3, off := s.t3, s.off // fetched by shmRefClock
	if t3.IsZero() || (t3.Hour() == 0 && t3.Minute() == 0 && t3.Second() == 0) {
		panic(fmt.Sprintf("PANIC: Fetched time from SHM and t3 is zero."))
	}

	// t3, off := time.Now(), time.Duration(0)

	// USE pathQualities MAP TO DERIVE RTT AND ASYM
	pq := s.pathQualities[snet.Fingerprint(p).String()]

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

	// logMsg := fmt.Sprintf(
	// 	"Generated timestamps: t0=%s, t1=%s, t2=%s, t3=%s | delays: d0=%v, d1=%v | offset: off=%v",
	// 	ts.t0.Format("15:04:05.000000"),
	// 	ts.t1.Format("15:04:05.000000"),
	// 	ts.t2.Format("15:04:05.000000"),
	// 	ts.t3.Format("15:04:05.000000"),
	// 	d0,
	// 	d1,
	// 	off,
	// )

	d0_sim := t1.Sub(t0).Seconds()
	d1_sim := t3.Sub(t2).Seconds()
	asym_sim := math.Abs(d0_sim - d1_sim)
	logMsg := fmt.Sprintf(
		"Reason: %s | FP: %s | Generated timestamps: t0=%s, t1=%s, t2=%s, t3=%s | Real delay asym: %v [micros] | Client delay asym: %v [micros] | Real delays: d0=%v, d1=%v | Client delays: d0=%v, d1=%v [ms] | d0+d1?=rtt: %t",
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

func assignPathQualities_v3() map[string]*PathQuality {
	return map[string]*PathQuality{
		// --- Very good paths (1–7): low RTT and jitter ---
		"d169d3fdcf2a7b89091fb85c61222c3cb3fed169bfa3994a49891042917553e7": {
			rttRange: []float64{23, 47},
			meanRTT:  24, jitter: 0.8, rng: rand.New(rand.NewSource(101)), asymBias: 0.52,
		}, // very good

		"5fb945f4d62fb9a9877489f10006d8f9c5a94f16656121e5eb78a0394a4f466e": {
			rttRange: []float64{24, 206},
			meanRTT:  25, jitter: 3.8, rng: rand.New(rand.NewSource(102)), asymBias: 0.51,
		}, // very good

		"6c4f86d4c33b5c2371494582f3224b1a3c0611709168c4f411f66c9fa27d744b": {
			rttRange: []float64{28, 50},
			meanRTT:  29, jitter: 1.0, rng: rand.New(rand.NewSource(103)), asymBias: 0.489,
		}, // very good

		"e1bf6b79f7219745133909e2a6fba731b92d001007cf81235cec08f3491bf196": {
			rttRange: []float64{29, 211},
			meanRTT:  30, jitter: 3.8, rng: rand.New(rand.NewSource(104)), asymBias: 0.492,
		}, // very good

		"b1319d5e0f1cc40a0d227ef38e03ee6d58bd352393cb48f6fbeea0d29032b036": {
			rttRange: []float64{29.5, 211.2},
			meanRTT:  30.3, jitter: 3.8, rng: rand.New(rand.NewSource(105)), asymBias: 0.502,
		}, // very good

		"a7a0a2901cccd754534d65374136b9829a7f81518309af79d87f0a560a14690c": {
			rttRange: []float64{39.8, 221.4},
			meanRTT:  40.5, jitter: 3.5, rng: rand.New(rand.NewSource(106)), asymBias: 0.494,
		}, // very good

		"02bf95b27afd43858f75306b7a77c5d548ab4618a12cb155775ead71345028f9": {
			rttRange: []float64{38.8, 60},
			meanRTT:  39.5, jitter: 0.9, rng: rand.New(rand.NewSource(107)), asymBias: 0.5012,
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

func assignPathQualities_exp1() map[string]*PathQuality {
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

		"1c91e925e44aebc06938ee1f41ad22d0c2c3691a877eaf57eb974088bcbd2e4e": {
			rttRange: []float64{20, 30},
			meanRTT:  25, jitter: 1.5, rng: rand.New(rand.NewSource(107)),
		},

		"4194af709787d03483b2c8f298a1e12fba6abb384a62ded807cb4cb7ee197fd7": {
			rttRange: []float64{40, 80},
			meanRTT:  60, jitter: 6, rng: rand.New(rand.NewSource(108)),
		},

		"877b3a86f6e88d9034423fde74939b9362cbc8ac368cec0be7eab8e5a9663c6e": {
			rttRange: []float64{50, 60},
			meanRTT:  55, jitter: 3, rng: rand.New(rand.NewSource(109)),
		},

		"108acbc64efbf5a04652c15db371381c37fba6646fcc000bde7b7877cc8db557": {
			rttRange: []float64{10, 20},
			meanRTT:  15, jitter: 1, rng: rand.New(rand.NewSource(110)),
		},

		"3496ccc115ec697f3e2027c1f2b70a364bc08cf49ecb6fcb335c3989fca26b02": {
			rttRange: []float64{100, 150},
			meanRTT:  130, jitter: 12, rng: rand.New(rand.NewSource(111)),
		},

		"1c1badde515e0cba50c1cbddeb792683884d22bcdfde8a6f4722d3be4dc9cb01": {
			rttRange: []float64{80, 90},
			meanRTT:  85, jitter: 5, rng: rand.New(rand.NewSource(112)),
		},

		"e488d5b91a7b360294f19bad4ddf644c41d7e47c82cb68256bbedc3eed9a6447": {
			rttRange: []float64{30, 40},
			meanRTT:  35, jitter: 1.5, rng: rand.New(rand.NewSource(113)),
		},

		"aa170b5d350ffdd234572466a5b9e557c6930d2d8669a3f12a813c9410a0c0c7": {
			rttRange: []float64{20, 50},
			meanRTT:  40, jitter: 4, rng: rand.New(rand.NewSource(114)),
		},

		"c138518cd3637650efb49977186c920375cc6b36f75923be55d04f8f9303e188": {
			rttRange: []float64{10, 100},
			meanRTT:  70, jitter: 15, rng: rand.New(rand.NewSource(115)),
		},

		"0e8ebf758be09f95eb8f9f5df6eadad00cece2207a4ced88eac43dfafc68597f": {
			rttRange: []float64{22, 39},
			meanRTT:  30, jitter: 2, rng: rand.New(rand.NewSource(116)),
		},

		"d8c9132fa9db80172bcd51547b87137986ee0533e49483ce4fcba9c277c26e4a": {
			rttRange: []float64{10, 15},
			meanRTT:  13, jitter: 0.8, rng: rand.New(rand.NewSource(117)),
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
