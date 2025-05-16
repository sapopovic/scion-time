package adjustments

import (
	"log/slog"
	"math"
	"time"

	"golang.org/x/sys/unix"

	"example.com/scion-time/base/logbase"
	"example.com/scion-time/base/unixutil"
)

type FuzzyPIController struct {
	StepThreshold          time.Duration
	prevOffset             float64
	prevDelta              float64
	reversals              int
	ReversalLimit          int
	p, i, freq, freqAddend float64
	KP, KI                 float64

	offsetWindow []float64
	WindowSize   int
	windowIndex  int
}

var _ Adjustment = (*FuzzyPIController)(nil)

func (c *FuzzyPIController) Do(offset time.Duration) {
	// ctx := context.Background()
	log := slog.Default()

	offsetSec := offset.Seconds()
	deltaOffset := offsetSec - c.prevOffset

	// Slope reversals
	if (deltaOffset > 0 && c.prevDelta < 0) || (deltaOffset < 0 && c.prevDelta > 0) {
		c.reversals++
	}
	c.prevDelta = deltaOffset
	c.prevOffset = offsetSec

	// Sliding window
	c.addToWindow(offsetSec)
	mean := c.meanOffset()
	stddev := c.stdDevOffset()

	// Step 1: Fuzzy base
	c.KP, c.KI = fuzzyInference(offsetSec, deltaOffset)

	// Step 2: Adaptive scaling
	kpScale, kiScale := 1.0, 1.0
	const biasThreshold = -0.00005
	if mean < biasThreshold {
		kiScale *= 1.2
	}
	if c.reversals >= c.ReversalLimit || stddev > 0.0002 {
		kpScale *= 0.9
		kiScale *= 0.95
		c.reversals = 0
	}
	const offsetTarget = 0.00006
	if math.Abs(mean) > offsetTarget {
		if math.Abs(deltaOffset) > 0.0002 {
			kpScale *= 1.15
			kiScale *= 0.9
		} else {
			kpScale *= 1.05
			kiScale *= 1.05
		}
	} else {
		kpScale *= 0.95
		kiScale *= 0.95
	}

	// Step 3: Apply scaling
	c.KP = clamp(c.KP*kpScale, 0.01, 0.5)
	c.KI = clamp(c.KI*kiScale, 0.01, 0.6)

	tx := unix.Timex{}
	_, err := unix.ClockAdjtime(unix.CLOCK_REALTIME, &tx)
	if err != nil {
		logbase.Fatal(log, "unix.ClockAdjtime failed", slog.Any("error", err))
	}
	freq := unixutil.FreqFromScaledPPM(tx.Freq)

	c.i += c.freqAddend * c.KI
	freq -= c.freqAddend - (c.freqAddend * c.KI)

	if c.StepThreshold != 0 && math.Abs(offsetSec) >= c.StepThreshold.Seconds() {
		tx = unix.Timex{
			Modes: unix.ADJ_SETOFFSET | unix.ADJ_NANO,
			Time:  unixutil.TimevalFromNsec(offset.Nanoseconds()),
		}
		_, err = unix.ClockAdjtime(unix.CLOCK_REALTIME, &tx)
		if err != nil {
			logbase.Fatal(log, "unix.ClockAdjtime failed", slog.Any("error", err))
		}
		c.i = 0 // Reset integral on hard clock step
		c.freqAddend = 0
		c.freq = 0
	} else {
		c.freqAddend = offsetSec * c.KP
		c.p = c.freqAddend
		freq += c.freqAddend
		tx = unix.Timex{
			Modes: unix.ADJ_FREQUENCY,
			Freq:  unixutil.ScaledPPMFromFreq(freq),
		}
		_, err = unix.ClockAdjtime(unix.CLOCK_REALTIME, &tx)
		if err != nil {
			logbase.Fatal(log, "unix.ClockAdjtime failed", slog.Any("error", err))
		}
		c.freq = freq
	}
}

func (c *FuzzyPIController) addToWindow(offset float64) {
	if len(c.offsetWindow) < c.WindowSize {
		c.offsetWindow = append(c.offsetWindow, offset)
	} else {
		c.offsetWindow[c.windowIndex] = offset
		c.windowIndex = (c.windowIndex + 1) % c.WindowSize
	}
}

func (c *FuzzyPIController) meanOffset() float64 {
	sum := 0.0
	for _, v := range c.offsetWindow {
		sum += v
	}
	return sum / float64(len(c.offsetWindow))
}

func (c *FuzzyPIController) stdDevOffset() float64 {
	mean := c.meanOffset()
	sumSq := 0.0
	for _, v := range c.offsetWindow {
		d := v - mean
		sumSq += d * d
	}
	return math.Sqrt(sumSq / float64(len(c.offsetWindow)))
}

func fuzzyInference(offset, delta float64) (float64, float64) {
	smallOffset := triangularMF(math.Abs(offset), 0.0, 0.001, 0.002)
	mediumOffset := triangularMF(math.Abs(offset), 0.001, 0.005, 0.01)
	largeOffset := trapezoidalMF(math.Abs(offset), 0.008, 0.01, 0.1, 0.2)

	negDelta := triangularMF(delta, -0.01, -0.005, 0.0)
	zeroDelta := triangularMF(delta, -0.001, 0.0, 0.001)
	posDelta := triangularMF(delta, 0.0, 0.005, 0.01)

	totalWeight := 0.0
	sumKP := 0.0
	sumKI := 0.0

	addRule := func(strength, kp, ki float64) {
		totalWeight += strength
		sumKP += strength * kp
		sumKI += strength * ki
	}

	addRule(smallOffset*negDelta, 0.02, 0.6)
	addRule(smallOffset*zeroDelta, 0.05, 0.4)
	addRule(smallOffset*posDelta, 0.08, 0.2)

	addRule(mediumOffset*negDelta, 0.1, 0.3)
	addRule(mediumOffset*zeroDelta, 0.15, 0.2)
	addRule(mediumOffset*posDelta, 0.25, 0.1)

	addRule(largeOffset*negDelta, 0.2, 0.05)
	addRule(largeOffset*zeroDelta, 0.3, 0.02)
	addRule(largeOffset*posDelta, 0.4, 0.01)

	if totalWeight == 0 {
		return 0.1, 0.1
	}

	kp := math.Min(math.Max(sumKP/totalWeight, 0.01), 0.4)
	ki := math.Min(math.Max(sumKI/totalWeight, 0.01), 0.6)
	return kp, ki
}

func triangularMF(x, a, b, c float64) float64 {
	if x <= a || x >= c {
		return 0
	}
	if x == b {
		return 1
	}
	if x < b {
		return (x - a) / (b - a)
	}
	return (c - x) / (c - b)
}

func trapezoidalMF(x, a, b, c, d float64) float64 {
	if x <= a || x >= d {
		return 0
	}
	if x >= b && x <= c {
		return 1
	}
	if x < b {
		return (x - a) / (b - a)
	}
	return (d - x) / (d - c)
}

func clamp(x, min, max float64) float64 {
	return math.Max(min, math.Min(max, x))
}

/*
package adjustments

import (
	"log/slog"
	"math"
	"time"

	"golang.org/x/sys/unix"

	"example.com/scion-time/base/logbase"
	"example.com/scion-time/base/unixutil"
)

type FuzzyPIController struct {
	StepThreshold          time.Duration
	prevOffset             float64
	prevDelta              float64
	reversals              int
	ReversalLimit          int
	p, i, freq, freqAddend float64
	KP, KI                 float64

	offsetWindow []float64
	WindowSize   int
	windowIndex  int
}

var _ Adjustment = (*FuzzyPIController)(nil)

func (c *FuzzyPIController) Do(offset time.Duration) {
	// ctx := context.Background()
	log := slog.Default()

	offsetSec := offset.Seconds()
	deltaOffset := offsetSec - c.prevOffset

	// Detect slope reversals
	if (deltaOffset > 0 && c.prevDelta < 0) || (deltaOffset < 0 && c.prevDelta > 0) {
		c.reversals++
	}
	c.prevDelta = deltaOffset
	c.prevOffset = offsetSec

	// Update sliding window
	c.addToWindow(offsetSec)
	mean := c.meanOffset()
	stddev := c.stdDevOffset()

	// Bias correction (not yet effective)
	const biasThreshold = -0.0001
	if mean < biasThreshold {
		c.KI = math.Min(c.KI*1.1, 0.6)
	}

	// Oscillation suppression
	if c.reversals >= c.ReversalLimit || stddev > 0.0002 {
		c.KP *= 0.9
		c.KI *= 0.9
		c.reversals = 0
	}

	// // Target zone adaptation
	// const offsetTarget = 0.00006
	// if math.Abs(mean) > offsetTarget {
	// 	if math.Abs(deltaOffset) > 0.0002 {
	// 		c.KP = math.Min(c.KP*1.15, 0.5)
	// 		c.KI = math.Max(c.KI*0.9, 0.01)
	// 	} else {
	// 		c.KP = math.Min(c.KP*1.05, 0.4)
	// 		c.KI = math.Min(c.KI*1.05, 0.5)
	// 	}
	// } else {
	// 	c.KP = math.Max(c.KP*0.95, 0.01)
	// 	c.KI = math.Max(c.KI*0.98, 0.01)
	// }

	// Fuzzy logic override (wipes out all above Ki logic)
	c.KP, c.KI = fuzzyInference(offsetSec, deltaOffset)

	tx := unix.Timex{}
	_, err := unix.ClockAdjtime(unix.CLOCK_REALTIME, &tx)
	if err != nil {
		logbase.Fatal(log, "unix.ClockAdjtime failed", slog.Any("error", err))
	}
	freq := unixutil.FreqFromScaledPPM(tx.Freq)

	c.i += c.freqAddend * c.KI
	freq -= c.freqAddend - (c.freqAddend * c.KI)

	if c.StepThreshold != 0 && math.Abs(offsetSec) >= c.StepThreshold.Seconds() {
		tx = unix.Timex{
			Modes: unix.ADJ_SETOFFSET | unix.ADJ_NANO,
			Time:  unixutil.TimevalFromNsec(offset.Nanoseconds()),
		}
		_, err = unix.ClockAdjtime(unix.CLOCK_REALTIME, &tx)
		if err != nil {
			logbase.Fatal(log, "unix.ClockAdjtime failed", slog.Any("error", err))
		}
		c.i = 0 // Reset integral on hard clock step
		c.freqAddend = 0
		c.freq = 0
	} else {
		c.freqAddend = offsetSec * c.KP
		c.p = c.freqAddend
		freq += c.freqAddend
		tx = unix.Timex{
			Modes: unix.ADJ_FREQUENCY,
			Freq:  unixutil.ScaledPPMFromFreq(freq),
		}
		_, err = unix.ClockAdjtime(unix.CLOCK_REALTIME, &tx)
		if err != nil {
			logbase.Fatal(log, "unix.ClockAdjtime failed", slog.Any("error", err))
		}
		c.freq = freq
	}
}

func (c *FuzzyPIController) addToWindow(offset float64) {
	if len(c.offsetWindow) < c.WindowSize {
		c.offsetWindow = append(c.offsetWindow, offset)
	} else {
		c.offsetWindow[c.windowIndex] = offset
		c.windowIndex = (c.windowIndex + 1) % c.WindowSize
	}
}

func (c *FuzzyPIController) meanOffset() float64 {
	sum := 0.0
	for _, v := range c.offsetWindow {
		sum += v
	}
	return sum / float64(len(c.offsetWindow))
}

func (c *FuzzyPIController) stdDevOffset() float64 {
	mean := c.meanOffset()
	sumSq := 0.0
	for _, v := range c.offsetWindow {
		d := v - mean
		sumSq += d * d
	}
	return math.Sqrt(sumSq / float64(len(c.offsetWindow)))
}

func fuzzyInference(offset, delta float64) (float64, float64) {
	smallOffset := triangularMF(math.Abs(offset), 0.0, 0.001, 0.002)
	mediumOffset := triangularMF(math.Abs(offset), 0.001, 0.005, 0.01)
	largeOffset := trapezoidalMF(math.Abs(offset), 0.008, 0.01, 0.1, 0.2)

	negDelta := triangularMF(delta, -0.01, -0.005, 0.0)
	zeroDelta := triangularMF(delta, -0.001, 0.0, 0.001)
	posDelta := triangularMF(delta, 0.0, 0.005, 0.01)

	totalWeight := 0.0
	sumKP := 0.0
	sumKI := 0.0

	addRule := func(strength, kp, ki float64) {
		totalWeight += strength
		sumKP += strength * kp
		sumKI += strength * ki
	}

	addRule(smallOffset*negDelta, 0.02, 0.6)
	addRule(smallOffset*zeroDelta, 0.05, 0.4)
	addRule(smallOffset*posDelta, 0.08, 0.2)

	addRule(mediumOffset*negDelta, 0.1, 0.3)
	addRule(mediumOffset*zeroDelta, 0.15, 0.2)
	addRule(mediumOffset*posDelta, 0.25, 0.1)

	addRule(largeOffset*negDelta, 0.2, 0.05)
	addRule(largeOffset*zeroDelta, 0.3, 0.02)
	addRule(largeOffset*posDelta, 0.4, 0.01)

	if totalWeight == 0 {
		return 0.1, 0.1
	}

	kp := math.Min(math.Max(sumKP/totalWeight, 0.01), 0.4)
	ki := math.Min(math.Max(sumKI/totalWeight, 0.01), 0.6)
	return kp, ki
}

func triangularMF(x, a, b, c float64) float64 {
	if x <= a || x >= c {
		return 0
	}
	if x == b {
		return 1
	}
	if x < b {
		return (x - a) / (b - a)
	}
	return (c - x) / (c - b)
}

func trapezoidalMF(x, a, b, c, d float64) float64 {
	if x <= a || x >= d {
		return 0
	}
	if x >= b && x <= c {
		return 1
	}
	if x < b {
		return (x - a) / (b - a)
	}
	return (d - x) / (d - c)
}

func clamp(x, min, max float64) float64 {
	return math.Max(min, math.Min(max, x))
}
*/

/*
package adjustments

import (
	"log/slog"
	"math"
	"time"

	"golang.org/x/sys/unix"

	"example.com/scion-time/base/logbase"
	"example.com/scion-time/base/unixutil"
)

type FuzzyPIController struct {
	StepThreshold          time.Duration
	prevOffset             float64
	prevDelta              float64
	reversals              int
	ReversalLimit          int
	p, i, freq, freqAddend float64
	KP, KI                 float64

	offsetWindow []float64
	WindowSize   int
	windowIndex  int
}

var _ Adjustment = (*FuzzyPIController)(nil)

func (c *FuzzyPIController) Do(offset time.Duration) {
	// ctx := context.Background()
	log := slog.Default()

	offsetSec := offset.Seconds()
	deltaOffset := offsetSec - c.prevOffset

	// Detect slope reversals
	if (deltaOffset > 0 && c.prevDelta < 0) || (deltaOffset < 0 && c.prevDelta > 0) {
		c.reversals++
	}
	c.prevDelta = deltaOffset
	c.prevOffset = offsetSec

	// Add to sliding window
	c.addToWindow(offsetSec)

	// Compute mean and std deviation
	mean := c.meanOffset()
	stddev := c.stdDevOffset()

	// Base gains from fuzzy logic
	c.KP, c.KI = fuzzyInference(offsetSec, deltaOffset)

	// Modifiers for adaptive tuning
	kpScale, kiScale := 1.0, 1.0

	const biasThreshold = -0.0001 // -100 µs
	if mean < biasThreshold {
		kiScale *= 1.1
	}

	if c.reversals >= c.ReversalLimit || stddev > 0.0002 {
		kpScale *= 0.9
		kiScale *= 0.9
		c.reversals = 0
	}

	const offsetTarget = 0.00006 // 60 µs
	if math.Abs(mean) > offsetTarget {
		if math.Abs(deltaOffset) > 0.0002 {
			kpScale *= 1.15
			kiScale *= 0.9
		} else {
			kpScale *= 1.05
			kiScale *= 1.05
		}
	} else {
		kpScale *= 0.95
		kiScale *= 0.98
	}

	// Apply all tuning at once
	c.KP = clamp(c.KP*kpScale, 0.01, 0.5)
	c.KI = clamp(c.KI*kiScale, 0.01, 0.6)

	tx := unix.Timex{}
	_, err := unix.ClockAdjtime(unix.CLOCK_REALTIME, &tx)
	if err != nil {
		logbase.Fatal(log, "unix.ClockAdjtime failed", slog.Any("error", err))
	}
	freq := unixutil.FreqFromScaledPPM(tx.Freq)

	c.i += c.freqAddend * c.KI
	freq -= c.freqAddend - (c.freqAddend * c.KI)

	if c.StepThreshold != 0 && math.Abs(offsetSec) >= c.StepThreshold.Seconds() {
		tx = unix.Timex{
			Modes: unix.ADJ_SETOFFSET | unix.ADJ_NANO,
			Time:  unixutil.TimevalFromNsec(offset.Nanoseconds()),
		}
		_, err = unix.ClockAdjtime(unix.CLOCK_REALTIME, &tx)
		if err != nil {
			logbase.Fatal(log, "unix.ClockAdjtime failed", slog.Any("error", err))
		}
		c.i = 0 // Reset integral on hard clock step
		c.freqAddend = 0
		c.freq = 0
	} else {
		c.freqAddend = offsetSec * c.KP
		c.p = c.freqAddend
		freq += c.freqAddend
		tx = unix.Timex{
			Modes: unix.ADJ_FREQUENCY,
			Freq:  unixutil.ScaledPPMFromFreq(freq),
		}
		_, err = unix.ClockAdjtime(unix.CLOCK_REALTIME, &tx)
		if err != nil {
			logbase.Fatal(log, "unix.ClockAdjtime failed", slog.Any("error", err))
		}
		c.freq = freq
	}
}

func (c *FuzzyPIController) addToWindow(offset float64) {
	if len(c.offsetWindow) < c.WindowSize {
		c.offsetWindow = append(c.offsetWindow, offset)
	} else {
		c.offsetWindow[c.windowIndex] = offset
		c.windowIndex = (c.windowIndex + 1) % c.WindowSize
	}
}

func (c *FuzzyPIController) meanOffset() float64 {
	sum := 0.0
	for _, v := range c.offsetWindow {
		sum += v
	}
	return sum / float64(len(c.offsetWindow))
}

func (c *FuzzyPIController) stdDevOffset() float64 {
	mean := c.meanOffset()
	sumSq := 0.0
	for _, v := range c.offsetWindow {
		d := v - mean
		sumSq += d * d
	}
	return math.Sqrt(sumSq / float64(len(c.offsetWindow)))
}

func fuzzyInference(offset, delta float64) (float64, float64) {
	smallOffset := triangularMF(math.Abs(offset), 0.0, 0.001, 0.002)
	mediumOffset := triangularMF(math.Abs(offset), 0.001, 0.005, 0.01)
	largeOffset := trapezoidalMF(math.Abs(offset), 0.008, 0.01, 0.1, 0.2)

	negDelta := triangularMF(delta, -0.01, -0.005, 0.0)
	zeroDelta := triangularMF(delta, -0.001, 0.0, 0.001)
	posDelta := triangularMF(delta, 0.0, 0.005, 0.01)

	totalWeight := 0.0
	sumKP := 0.0
	sumKI := 0.0

	addRule := func(strength, kp, ki float64) {
		totalWeight += strength
		sumKP += strength * kp
		sumKI += strength * ki
	}

	addRule(smallOffset*negDelta, 0.02, 0.6)
	addRule(smallOffset*zeroDelta, 0.05, 0.4)
	addRule(smallOffset*posDelta, 0.08, 0.2)

	addRule(mediumOffset*negDelta, 0.1, 0.3)
	addRule(mediumOffset*zeroDelta, 0.15, 0.2)
	addRule(mediumOffset*posDelta, 0.25, 0.1)

	addRule(largeOffset*negDelta, 0.2, 0.05)
	addRule(largeOffset*zeroDelta, 0.3, 0.02)
	addRule(largeOffset*posDelta, 0.4, 0.01)

	if totalWeight == 0 {
		return 0.1, 0.1
	}

	kp := math.Min(math.Max(sumKP/totalWeight, 0.01), 0.4)
	ki := math.Min(math.Max(sumKI/totalWeight, 0.01), 0.6)
	return kp, ki
}

func triangularMF(x, a, b, c float64) float64 {
	if x <= a || x >= c {
		return 0
	}
	if x == b {
		return 1
	}
	if x < b {
		return (x - a) / (b - a)
	}
	return (c - x) / (c - b)
}

func trapezoidalMF(x, a, b, c, d float64) float64 {
	if x <= a || x >= d {
		return 0
	}
	if x >= b && x <= c {
		return 1
	}
	if x < b {
		return (x - a) / (b - a)
	}
	return (d - x) / (d - c)
}

func clamp(x, min, max float64) float64 {
	return math.Max(min, math.Min(max, x))
}
*/

/*
package adjustments

import (
	"log/slog"
	"math"
	"time"

	"golang.org/x/sys/unix"

	"example.com/scion-time/base/logbase"
	"example.com/scion-time/base/unixutil"
)

type FuzzyPIController struct {
	StepThreshold          time.Duration
	prevOffset             float64
	prevDelta              float64
	reversals              int
	ReversalLimit          int
	p, i, freq, freqAddend float64
	KP, KI                 float64
}

var _ Adjustment = (*FuzzyPIController)(nil)

func (c *FuzzyPIController) Do(offset time.Duration) {
	// ctx := context.Background()
	log := slog.Default()

	offsetSec := offset.Seconds()
	deltaOffset := offsetSec - c.prevOffset

	// Detect oscillation via slope reversals
	if (deltaOffset > 0 && c.prevDelta < 0) || (deltaOffset < 0 && c.prevDelta > 0) {
		c.reversals++
	}

	c.prevDelta = deltaOffset
	c.prevOffset = offsetSec

	// If oscillation detected, reduce gains
	if c.reversals >= c.ReversalLimit {
		c.KP *= 0.9
		c.KI *= 0.9
		c.reversals = 0
	}

	c.KP, c.KI = fuzzyInference(offsetSec, deltaOffset)
	adjustGainsForTarget(offsetSec, deltaOffset, &c.KP, &c.KI)

	tx := unix.Timex{}
	_, err := unix.ClockAdjtime(unix.CLOCK_REALTIME, &tx)
	if err != nil {
		logbase.Fatal(log, "unix.ClockAdjtime failed", slog.Any("error", err))
	}
	freq := unixutil.FreqFromScaledPPM(tx.Freq)

	c.i += c.freqAddend * c.KI
	freq -= c.freqAddend - (c.freqAddend * c.KI)

	if c.StepThreshold != 0 && math.Abs(offsetSec) >= c.StepThreshold.Seconds() {
		tx = unix.Timex{
			Modes: unix.ADJ_SETOFFSET | unix.ADJ_NANO,
			Time:  unixutil.TimevalFromNsec(offset.Nanoseconds()),
		}
		_, err = unix.ClockAdjtime(unix.CLOCK_REALTIME, &tx)
		if err != nil {
			logbase.Fatal(log, "unix.ClockAdjtime failed", slog.Any("error", err))
		}
		c.freqAddend = 0
		c.freq = 0
	} else {
		c.freqAddend = offsetSec * c.KP
		c.p = c.freqAddend
		freq += c.freqAddend
		tx = unix.Timex{
			Modes: unix.ADJ_FREQUENCY,
			Freq:  unixutil.ScaledPPMFromFreq(freq),
		}
		_, err = unix.ClockAdjtime(unix.CLOCK_REALTIME, &tx)
		if err != nil {
			logbase.Fatal(log, "unix.ClockAdjtime failed", slog.Any("error", err))
		}
		c.freq = freq
	}
}

func adjustGainsForTarget(offsetSec, deltaOffset float64, kp, ki *float64) {
	const offsetTarget = 0.0001

	if math.Abs(offsetSec) > offsetTarget {
		if math.Abs(deltaOffset) > 0.0002 {
			*kp = math.Min(*kp*1.15, 0.5)
			*ki = math.Max(*ki*0.9, 0.01)
		} else {
			*kp = math.Min(*kp*1.05, 0.4)
			*ki = math.Min(*ki*1.05, 0.5)
		}
	} else {
		*kp = math.Max(*kp*0.95, 0.01)
		*ki = math.Max(*ki*0.98, 0.01)
	}
}

func fuzzyInference(offset, delta float64) (float64, float64) {
	smallOffset := triangularMF(math.Abs(offset), 0.0, 0.001, 0.002)
	mediumOffset := triangularMF(math.Abs(offset), 0.001, 0.005, 0.01)
	largeOffset := trapezoidalMF(math.Abs(offset), 0.008, 0.01, 0.1, 0.2)

	negDelta := triangularMF(delta, -0.01, -0.005, 0.0)
	zeroDelta := triangularMF(delta, -0.001, 0.0, 0.001)
	posDelta := triangularMF(delta, 0.0, 0.005, 0.01)

	totalWeight := 0.0
	sumKP := 0.0
	sumKI := 0.0

	addRule := func(strength, kp, ki float64) {
		totalWeight += strength
		sumKP += strength * kp
		sumKI += strength * ki
	}

	addRule(smallOffset*negDelta, 0.02, 0.6)
	addRule(smallOffset*zeroDelta, 0.05, 0.4)
	addRule(smallOffset*posDelta, 0.08, 0.2)

	addRule(mediumOffset*negDelta, 0.1, 0.3)
	addRule(mediumOffset*zeroDelta, 0.15, 0.2)
	addRule(mediumOffset*posDelta, 0.25, 0.1)

	addRule(largeOffset*negDelta, 0.2, 0.05)
	addRule(largeOffset*zeroDelta, 0.3, 0.02)
	addRule(largeOffset*posDelta, 0.4, 0.01)

	if totalWeight == 0 {
		return 0.1, 0.1
	}

	kp := math.Min(math.Max(sumKP/totalWeight, 0.01), 0.4)
	ki := math.Min(math.Max(sumKI/totalWeight, 0.01), 0.6)
	return kp, ki
}

func triangularMF(x, a, b, c float64) float64 {
	if x <= a || x >= c {
		return 0
	}
	if x == b {
		return 1
	}
	if x < b {
		return (x - a) / (b - a)
	}
	return (c - x) / (c - b)
}

func trapezoidalMF(x, a, b, c, d float64) float64 {
	if x <= a || x >= d {
		return 0
	}
	if x >= b && x <= c {
		return 1
	}
	if x < b {
		return (x - a) / (b - a)
	}
	return (d - x) / (d - c)
}
*/
/*
package adjustments

import (
	"log/slog"
	"math"
	"time"

	"golang.org/x/sys/unix"

	"example.com/scion-time/base/logbase"
	"example.com/scion-time/base/unixutil"
)

type FuzzyPIController struct {
	StepThreshold          time.Duration
	prevOffset             float64
	p, i, freq, freqAddend float64
	KP, KI                 float64
}

var _ Adjustment = (*FuzzyPIController)(nil)

func (c *FuzzyPIController) Do(offset time.Duration) {
	// ctx := context.Background()
	log := slog.Default()

	offsetSec := offset.Seconds()
	deltaOffset := offsetSec - c.prevOffset
	c.prevOffset = offsetSec

	c.KP, c.KI = fuzzyInference(offsetSec, deltaOffset)

	tx := unix.Timex{}
	_, err := unix.ClockAdjtime(unix.CLOCK_REALTIME, &tx)
	if err != nil {
		logbase.Fatal(log, "unix.ClockAdjtime failed", slog.Any("error", err))
	}
	freq := unixutil.FreqFromScaledPPM(tx.Freq)

	c.i += c.freqAddend * c.KI
	freq -= c.freqAddend - (c.freqAddend * c.KI)

	if c.StepThreshold != 0 && math.Abs(offsetSec) >= c.StepThreshold.Seconds() {
		tx = unix.Timex{
			Modes: unix.ADJ_SETOFFSET | unix.ADJ_NANO,
			Time:  unixutil.TimevalFromNsec(offset.Nanoseconds()),
		}
		_, err = unix.ClockAdjtime(unix.CLOCK_REALTIME, &tx)
		if err != nil {
			logbase.Fatal(log, "unix.ClockAdjtime failed", slog.Any("error", err))
		}
		c.freqAddend = 0
		c.freq = 0
	} else {
		c.freqAddend = offsetSec * c.KP
		c.p = c.freqAddend
		freq += c.freqAddend
		tx = unix.Timex{
			Modes: unix.ADJ_FREQUENCY,
			Freq:  unixutil.ScaledPPMFromFreq(freq),
		}
		_, err = unix.ClockAdjtime(unix.CLOCK_REALTIME, &tx)
		if err != nil {
			logbase.Fatal(log, "unix.ClockAdjtime failed", slog.Any("error", err))
		}
		c.freq = freq
	}
}

func fuzzyInference(offset, delta float64) (float64, float64) {
	smallOffset := triangularMF(math.Abs(offset), 0.0, 0.001, 0.002)
	mediumOffset := triangularMF(math.Abs(offset), 0.001, 0.005, 0.01)
	largeOffset := trapezoidalMF(math.Abs(offset), 0.008, 0.01, 0.1, 0.2)

	negDelta := triangularMF(delta, -0.01, -0.005, 0.0)
	zeroDelta := triangularMF(delta, -0.001, 0.0, 0.001)
	posDelta := triangularMF(delta, 0.0, 0.005, 0.01)

	totalWeight := 0.0
	sumKP := 0.0
	sumKI := 0.0

	addRule := func(strength, kp, ki float64) {
		totalWeight += strength
		sumKP += strength * kp
		sumKI += strength * ki
	}

	addRule(smallOffset*negDelta, 0.02, 0.6)
	addRule(smallOffset*zeroDelta, 0.05, 0.4)
	addRule(smallOffset*posDelta, 0.08, 0.2)

	addRule(mediumOffset*negDelta, 0.1, 0.3)
	addRule(mediumOffset*zeroDelta, 0.15, 0.2)
	addRule(mediumOffset*posDelta, 0.25, 0.1)

	addRule(largeOffset*negDelta, 0.2, 0.05)
	addRule(largeOffset*zeroDelta, 0.3, 0.02)
	addRule(largeOffset*posDelta, 0.4, 0.01)

	if totalWeight == 0 {
		return 0.1, 0.1 // fallback
	}

	kp := math.Min(math.Max(sumKP/totalWeight, 0.01), 0.4)
	ki := math.Min(math.Max(sumKI/totalWeight, 0.01), 0.6)
	return kp, ki
}

func triangularMF(x, a, b, c float64) float64 {
	if x <= a || x >= c {
		return 0
	}
	if x == b {
		return 1
	}
	if x < b {
		return (x - a) / (b - a)
	}
	return (c - x) / (c - b)
}

func trapezoidalMF(x, a, b, c, d float64) float64 {
	if x <= a || x >= d {
		return 0
	}
	if x >= b && x <= c {
		return 1
	}
	if x < b {
		return (x - a) / (b - a)
	}
	return (d - x) / (d - c)
}
*/
