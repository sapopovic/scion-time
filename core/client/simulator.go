package client

import (
	"bytes"
	"context"
	"crypto/rand"
	"fmt"
	"log/slog"
	"math/big"
	"os"
	"strconv"
	"strings"
	"time"

	"example.com/scion-time/base/logbase"
	"example.com/scion-time/driver/shm"
	"github.com/pelletier/go-toml/v2"
)

// We have one simulator per path that extracts information about the given path from the config
// The config will contain jitter, asymmetry information about each path.
// There will be multiple configs: high jitter/low asymmetry, high jitter/high asymmetry, path failure
// We have one simulator per path that extracts information about the given path from the config after time x
type Simulator struct {
	pathConfig string
	rttMin     int
	rttMax     int
	asymRange  time.Duration
	cfg        SimulatorConfig
	log        *slog.Logger
	SHM        ReferenceClock
	ctx        context.Context
}

// In config: shm_reference_clock = ["ntpshm"]
type SimulatorConfig struct {
	SHMReferenceClock []string `toml:"shm_reference_clock,omitempty"`
}

func NewSimulator(configFile string, ctx context.Context) *Simulator {
	log := slog.Default()
	cfg := loadConfig(configFile)
	refClock := make([]ReferenceClock, 1)

	for _, s := range cfg.SHMReferenceClock { // we only have one
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

	return &Simulator{log: log, cfg: cfg, SHM: refClock[0]}
}

func (s Simulator) generateTimeStamps() TimeStamps {

	// Step 1: Fetch current GNSS time and client offset
	t2, off, err := s.SHM.MeasureClockOffset(s.ctx)
	if err != nil {
		panic(fmt.Sprintf("error fetching clock offset: %v", err))
	}
	t1 := t2 // zero processing delay

	// Step 2: Sample RTT
	rtt := time.Duration(SecureRandomInt(s.rttMin, s.rttMax))

	// Step 3: Sample asymmetry independently from RTT range
	asymMin := -int64(rtt)
	asymMax := int64(rtt)
	asym := time.Duration(SecureRandomInt(int(asymMin), int(asymMax)))

	// Step 4: Compute one-way delays (in GNSS time)
	d0 := (rtt + asym) / 2
	d1 := (rtt - asym) / 2

	// Step 5: Reconstruct timestamps
	t0 := t1.Add(-d0).Add(off) // client clock: request sent
	t3 := t2.Add(d1).Add(off)  // client clock: response received

	return TimeStamps{
		t0: t0, // client send (local clock)
		t1: t1, // server receive (GNSS)
		t2: t2, // server send (GNSS)
		t3: t3, // client receive (local clock)
	}

}

// Returns a secure random int in [min, max)
func SecureRandomInt(min, max int) int64 {
	diff := big.NewInt(int64(max - min))
	n, _ := rand.Int(rand.Reader, diff)
	rtt := n.Int64() + int64(min)

	return rtt
}

func loadConfig(configFile string) SimulatorConfig {
	raw, err := os.ReadFile(configFile)
	if err != nil {
		logbase.Fatal(slog.Default(), "failed to load configuration", slog.Any("error", err))
	}
	var cfg SimulatorConfig
	err = toml.NewDecoder(bytes.NewReader(raw)).DisallowUnknownFields().Decode(&cfg)
	if err != nil {
		logbase.Fatal(slog.Default(), "failed to decode configuration", slog.Any("error", err))
	}
	return cfg
}
