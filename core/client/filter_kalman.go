// Package kalman implements a 2D Kalman filter for NTP time synchronization.
package client

import (
	"context"
	"log/slog"
	"time"
)

// KalmanFilter represents a Kalman filter for tracking offset and frequency.
type KalmanFilter struct {
	log           *slog.Logger
	logCtx        context.Context
	State         [2]float64
	Cov           [2][2]float64
	LastUpdate    time.Time
	StartTime     time.Time
	Wander        float64 // frequency wander (process noise)
	R             float64 // measurement noise
	WarmupOffsets []float64
	Initialized   bool
	rttSamples    []float64
}

// NewKalmanFilter initializes the Kalman filter with config-derived constants.
func NewKalmanFilter(log *slog.Logger) *KalmanFilter {
	log.Info("Kalman Filter activated")
	const offsetMean = 150e-6    // 100 microseconds
	const offsetVariance = 1e-12 // example variance for initial offset
	wander := 1e-8               // taken from config
	freqUncertainty := 100e-6    // taken from config, 100 ppm
	t := time.Now()

	return &KalmanFilter{
		State: [2]float64{
			offsetMean,
			0.0, // initial frequency
		},
		Cov: [2][2]float64{
			{offsetVariance, 0},
			{0, freqUncertainty * freqUncertainty},
		},
		LastUpdate:    t,
		StartTime:     t,
		Wander:        wander, // e.g., 1e-8
		R:             1e-6,   // initial RTT variance, may adapt over time
		log:           log,
		logCtx:        context.Background(),
		WarmupOffsets: make([]float64, 0, 1000),
		Initialized:   false,
		rttSamples:    make([]float64, 0, 1000),
	}
}

func matMul(a, b [2][2]float64) [2][2]float64 {
	return [2][2]float64{
		{
			a[0][0]*b[0][0] + a[0][1]*b[1][0],
			a[0][0]*b[0][1] + a[0][1]*b[1][1],
		},
		{
			a[1][0]*b[0][0] + a[1][1]*b[1][0],
			a[1][0]*b[0][1] + a[1][1]*b[1][1],
		},
	}
}

func transpose(a [2][2]float64) [2][2]float64 {
	return [2][2]float64{
		{a[0][0], a[1][0]},
		{a[0][1], a[1][1]},
	}
}

func add(a, b [2][2]float64) [2][2]float64 {
	return [2][2]float64{
		{a[0][0] + b[0][0], a[0][1] + b[0][1]},
		{a[1][0] + b[1][0], a[1][1] + b[1][1]},
	}
}

func symmetrize(m [2][2]float64) [2][2]float64 {
	return [2][2]float64{
		{m[0][0], 0.5 * (m[0][1] + m[1][0])},
		{0.5 * (m[0][1] + m[1][0]), m[1][1]},
	}
}

// Predict extrapolates the filter forward to the given time.
func (kf *KalmanFilter) Predict(to time.Time) {
	dt := to.Sub(kf.LastUpdate).Seconds()
	if dt <= 0 {
		return
	}

	// Predict state
	kf.State[0] += dt * kf.State[1]

	// Transition matrix
	F := [2][2]float64{
		{1, dt},
		{0, 1},
	}

	dt2 := dt * dt
	dt3 := dt2 * dt
	v := kf.Wander
	Q := [2][2]float64{
		{v * dt3 / 3, v * dt2 / 2},
		{v * dt2 / 2, v * dt},
	}

	kf.Cov = add(matMul(matMul(F, kf.Cov), transpose(F)), Q)
	kf.Cov = symmetrize(kf.Cov)
	kf.LastUpdate = to
}

// Update incorporates a new offset measurement.
func (kf *KalmanFilter) Update(offsetMeas float64, t time.Time) {
	kf.Predict(t)

	y := offsetMeas - kf.State[0] // innovation
	S := kf.Cov[0][0] + kf.R      // innovation covariance

	K := [2]float64{
		kf.Cov[0][0] / S,
		kf.Cov[1][0] / S,
	}

	kf.State[0] += K[0] * y
	kf.State[1] += K[1] * y

	P := kf.Cov
	for i := 0; i < 2; i++ {
		for j := 0; j < 2; j++ {
			kf.Cov[i][j] -= K[i] * []float64{1, 0}[j] * P[0][j]
		}
	}
	kf.Cov = symmetrize(kf.Cov)
}

func computeStats(samples []float64) (mean, variance float64) {
	n := float64(len(samples))
	if n == 0 {
		return 0, 1e-12 // fallback
	}
	for _, x := range samples {
		mean += x
	}
	mean /= n
	for _, x := range samples {
		d := x - mean
		variance += d * d
	}
	variance /= n
	if variance < 1e-20 {
		variance = 1e-20 // floor to avoid divide-by-zero
	}
	return
}

func rttVariance(samples []float64) float64 {
	if len(samples) == 0 {
		return 1e-6
	}
	var sum, sumSq float64
	for _, x := range samples {
		sum += x
		sumSq += x * x
	}
	n := float64(len(samples))
	mean := sum / n
	variance := (sumSq / n) - (mean * mean)
	if variance < 1e-12 {
		variance = 1e-12
	}
	return variance
}

// Do performs a full NTP measurement update using 4 timestamps.
func (kf *KalmanFilter) Do(t1, t2, t3, t4 time.Time) time.Duration {
	rtt := t4.Sub(t1).Seconds() - t3.Sub(t2).Seconds()

	if rtt > 0 {
		kf.rttSamples = append(kf.rttSamples, rtt)
		if len(kf.rttSamples) > 1000 {
			kf.rttSamples = kf.rttSamples[1:] // slide window
		}
		kf.R = rttVariance(kf.rttSamples) / 4
	}

	offset := (t2.Sub(t1).Seconds() + t3.Sub(t4).Seconds()) / 2

	if !kf.Initialized {
		if time.Since(kf.StartTime) < 1*time.Minute {
			kf.WarmupOffsets = append(kf.WarmupOffsets, offset)
			if kf.log != nil {
				kf.log.LogAttrs(kf.logCtx, slog.LevelDebug, "warm-up (raw) response",
					slog.Float64("Offset [µs]", offset*1e6),
				)
			}
			return time.Duration(offset * float64(time.Second))
		} else {
			// Finalize filter after warm-up
			mean, variance := computeStats(kf.WarmupOffsets)
			kf.State[0] = mean
			kf.State[1] = 0.0
			kf.Cov[0][0] = variance
			kf.Cov[1][1] = 100e-6 * 100e-6
			kf.LastUpdate = t4
			kf.Initialized = true

			if kf.log != nil {
				kf.log.LogAttrs(kf.logCtx, slog.LevelInfo, "kalman filter initialized",
					slog.Float64("mean [µs]", mean*1e6),
					slog.Float64("variance", variance),
					slog.Int("Nb of samples", len(kf.WarmupOffsets)),
				)
			}
		}
	}

	kf.Update(offset, t4)

	if kf.log != nil {
		kf.log.LogAttrs(kf.logCtx, slog.LevelDebug, "filtered response",
			slog.Float64("Filtered Offset [µs]", kf.Offset()*1e6),
		)
	}

	return time.Duration(kf.Offset() * float64(time.Second))

}

// Offset returns the current estimated offset.
func (kf *KalmanFilter) Offset() float64 {
	return kf.State[0]
}

// Frequency returns the current estimated frequency difference.
func (kf *KalmanFilter) Frequency() float64 {
	return kf.State[1]
}

func (kf *KalmanFilter) Reset() {
	now := time.Now()
	const offsetMean = 100e-6      // 100 microseconds
	const offsetVariance = 1e-12   // Reasonable starting variance
	const freqUncertainty = 100e-6 // From ntpd-rs default

	kf.State = [2]float64{
		offsetMean, // reset to initial offset
		0.0,        // reset frequency to 0.0
	}
	kf.Cov = [2][2]float64{
		{offsetVariance, 0},
		{0, freqUncertainty * freqUncertainty},
	}
	kf.LastUpdate = now
	kf.StartTime = now
}
