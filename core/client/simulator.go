package client

import (
	"context"
	"crypto/rand"
	"fmt"
	"log/slog"
	"math/big"
	"strconv"
	"strings"
	"time"

	"example.com/scion-time/base/logbase"
	"example.com/scion-time/driver/shm"
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
	ctx           context.Context
	pathQualities map[string]*PathQuality
}

func NewSimulator(simRefClock []string) *Simulator {
	log := slog.Default()
	refClock := make([]ReferenceClock, 1)

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

	pqs := assignPathQualities()

	return &Simulator{log: log, SHM: refClock[0], pathQualities: pqs}
}

func assignPathQualities() map[string]*PathQuality {
	return map[string]*PathQuality{
		"1c91e925e44aebc06938ee1f41ad22d0c2c3691a877eaf57eb974088bcbd2e4e": {
			rttRange:  []float64{10, 20},
			asymRange: []float64{0.1, 0.3},
			seed:      101,
		},
		"4194af709787d03483b2c8f298a1e12fba6abb384a62ded807cb4cb7ee197fd7": {
			rttRange:  []float64{15, 25},
			asymRange: []float64{0.2, 0.4},
			seed:      102,
		},
		"877b3a86f6e88d9034423fde74939b9362cbc8ac368cec0be7eab8e5a9663c6e": {
			rttRange:  []float64{12, 22},
			asymRange: []float64{0.15, 0.35},
			seed:      103,
		},
		"108acbc64efbf5a04652c15db371381c37fba6646fcc000bde7b7877cc8db557": {
			rttRange:  []float64{18, 28},
			asymRange: []float64{0.1, 0.25},
			seed:      104,
		},
		"3496ccc115ec697f3e2027c1f2b70a364bc08cf49ecb6fcb335c3989fca26b02": {
			rttRange:  []float64{11, 21},
			asymRange: []float64{0.05, 0.2},
			seed:      105,
		},
		"d169d3fdcf2a7b89091fb85c61222c3cb3fed169bfa3994a49891042917553e7": {
			rttRange:  []float64{20, 30},
			asymRange: []float64{0.2, 0.5},
			seed:      106,
		},
		"1c1badde515e0cba50c1cbddeb792683884d22bcdfde8a6f4722d3be4dc9cb01": {
			rttRange:  []float64{9, 16},
			asymRange: []float64{0.05, 0.15},
			seed:      107,
		},
		"6c4f86d4c33b5c2371494582f3224b1a3c0611709168c4f411f66c9fa27d744b": {
			rttRange:  []float64{13, 18},
			asymRange: []float64{0.1, 0.3},
			seed:      108,
		},
		"e488d5b91a7b360294f19bad4ddf644c41d7e47c82cb68256bbedc3eed9a6447": {
			rttRange:  []float64{22, 35},
			asymRange: []float64{0.25, 0.45},
			seed:      109,
		},
		"a7a0a2901cccd754534d65374136b9829a7f81518309af79d87f0a560a14690c": {
			rttRange:  []float64{8, 14},
			asymRange: []float64{0.05, 0.1},
			seed:      110,
		},
		"aa170b5d350ffdd234572466a5b9e557c6930d2d8669a3f12a813c9410a0c0c7": {
			rttRange:  []float64{17, 27},
			asymRange: []float64{0.3, 0.6},
			seed:      111,
		},
		"c138518cd3637650efb49977186c920375cc6b36f75923be55d04f8f9303e188": {
			rttRange:  []float64{19, 29},
			asymRange: []float64{0.2, 0.4},
			seed:      112,
		},
		"5fb945f4d62fb9a9877489f10006d8f9c5a94f16656121e5eb78a0394a4f466e": {
			rttRange:  []float64{16, 23},
			asymRange: []float64{0.1, 0.25},
			seed:      113,
		},
		"02bf95b27afd43858f75306b7a77c5d548ab4618a12cb155775ead71345028f9": {
			rttRange:  []float64{21, 32},
			asymRange: []float64{0.3, 0.5},
			seed:      114,
		},
		"0e8ebf758be09f95eb8f9f5df6eadad00cece2207a4ced88eac43dfafc68597f": {
			rttRange:  []float64{12, 19},
			asymRange: []float64{0.1, 0.2},
			seed:      115,
		},
		"e1bf6b79f7219745133909e2a6fba731b92d001007cf81235cec08f3491bf196": {
			rttRange:  []float64{14, 26},
			asymRange: []float64{0.15, 0.3},
			seed:      116,
		},
		"d8c9132fa9db80172bcd51547b87137986ee0533e49483ce4fcba9c277c26e4a": {
			rttRange:  []float64{10, 15},
			asymRange: []float64{0.05, 0.1},
			seed:      117,
		},
		"b1319d5e0f1cc40a0d227ef38e03ee6d58bd352393cb48f6fbeea0d29032b036": {
			rttRange:  []float64{18, 24},
			asymRange: []float64{0.2, 0.35},
			seed:      118,
		},
	}
}

func (s Simulator) generateTimeStamps() TimeStamps {

	// Step 1: Fetch client receive time (local clock) and offset to GNSS
	t3, off, err := s.SHM.MeasureClockOffset(s.ctx) // t3 = local clock, offset = GNSS - local
	if err != nil {
		panic(fmt.Sprintf("error fetching clock offset: %v", err))
	}

	// USE pathQualities MAP TO DERIVE RTT AND ASYM

	// Step 2: Sample RTT
	rtt := time.Duration(SecureRandomInt(s.rttMin, s.rttMax))

	// Step 3: Sample asymmetry (independent of RTT)
	asymMin := -int64(rtt)
	asymMax := int64(rtt)
	asym := time.Duration(SecureRandomInt(int(asymMin), int(asymMax)))

	// Step 4: Compute one-way delays (true delays, gnss time perspective)
	d1 := rtt/2 - asym/2 // client <- server
	d0 := rtt/2 + asym/2 // client -> server

	// Step 5: Reconstruct the rest
	t2 := t3.Add(-d1).Add(off)  // GNSS time (server send)
	t1 := t2                    // no processing delay at server
	t0 := t1.Add(-d0).Add(-off) // local clock (client send)

	return TimeStamps{
		t0: t0, // client send (local)
		t1: t1, // server recv (GNSS)
		t2: t2, // server send (GNSS)
		t3: t3, // client recv (local)
	}

}

// Returns a secure random int in [min, max)
func SecureRandomInt(min, max int) int64 {
	diff := big.NewInt(int64(max - min))
	n, _ := rand.Int(rand.Reader, diff)
	rtt := n.Int64() + int64(min)

	return rtt
}
