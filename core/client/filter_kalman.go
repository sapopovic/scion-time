package client

import (
	"log/slog"
	"time"

	"example.com/scion-time/core/measurements"
	"example.com/scion-time/net/ntp"
)

type KalmanFilter struct {
	offset float64
	skew   float64
	P      [2][2]float64
	Q      [2][2]float64
	R      float64
}

var _ measurements.Filter = (*KalmanFilter)(nil)

func NewKalmanFilter(log *slog.Logger) *KalmanFilter {
	initialOffset := 0.0
	initialSkew := 0.0
	initialP := [2][2]float64{
		{10000, 0},
		{0, 1},
	}
	processNoiseOffset := 1.0
	processNoiseSkew := 1e-4
	measurementNoise := 4.0

	kf := &KalmanFilter{
		offset: initialOffset,
		skew:   initialSkew,
		R:      measurementNoise,
		P:      initialP,
		Q: [2][2]float64{
			{processNoiseOffset, 0},
			{0, processNoiseSkew},
		},
	}

	log.Info("Kalman Filter activated")
	return kf
}

func (kf *KalmanFilter) Predict(dt float64) {
	newOffset := kf.offset + kf.skew*dt
	newSkew := kf.skew

	kf.offset = newOffset
	kf.skew = newSkew

	p00 := kf.P[0][0]
	p01 := kf.P[0][1]
	p10 := kf.P[1][0]
	p11 := kf.P[1][1]

	a00 := p00 + dt*p10
	a01 := p01 + dt*p11
	a10 := p10
	a11 := p11

	kf.P[0][0] = a00 + dt*a01
	kf.P[0][1] = a01
	kf.P[1][0] = a10 + dt*a11
	kf.P[1][1] = a11

	kf.P[0][0] += kf.Q[0][0]
	kf.P[0][1] += kf.Q[0][1]
	kf.P[1][0] += kf.Q[1][0]
	kf.P[1][1] += kf.Q[1][1]
}

func (kf *KalmanFilter) Update(measurement float64) {
	y := measurement - kf.offset
	S := kf.P[0][0] + kf.R

	K0 := kf.P[0][0] / S
	K1 := kf.P[1][0] / S

	kf.offset += K0 * y
	kf.skew += K1 * y

	p00 := kf.P[0][0]
	p01 := kf.P[0][1]
	p10 := kf.P[1][0]
	p11 := kf.P[1][1]

	newP00 := p00 - K0*p00
	newP01 := p01 - K0*p01
	newP10 := p10 - K1*p00
	newP11 := p11 - K1*p01

	avgOffDiag := (newP01 + newP10) / 2
	kf.P[0][0] = newP00
	kf.P[0][1] = avgOffDiag
	kf.P[1][0] = avgOffDiag
	kf.P[1][1] = newP11
}

func (kf *KalmanFilter) SetMeasurementNoise(newR float64) {
	kf.R = newR
}

func (kf *KalmanFilter) GetState() (offset, skew float64) {
	return kf.offset, kf.skew
}

// Do processes one new 4-timestamp measurement.
// It computes the raw offset from (cTxTime, sRxTime, sTxTime, cRxTime),
// runs Kalman Predict+Update, and returns the filtered offset.
func (kf *KalmanFilter) Do(cTxTime, sRxTime, sTxTime, cRxTime time.Time) time.Duration {

	rawOffset := ntp.ClockOffset(cTxTime, sRxTime, sTxTime, cRxTime)

	// Predict step (you can pass actual elapsed dt between measurements if you track it)
	kf.Predict(1.0)

	// Update step with raw offset measurement (convert Duration to float64 nanoseconds)
	kf.Update(float64(rawOffset.Nanoseconds())) // SECONDS?

	// Return the filtered offset (still as time.Duration)
	filteredOffset := time.Duration(kf.offset)
	return filteredOffset
}

// Reset re-initializes the filter state (optional, e.g., after large jumps or resync).
func (kf *KalmanFilter) Reset() {
	kf.offset = 0.0
	kf.skew = 0.0
	kf.P = [2][2]float64{
		{10000, 0},
		{0, 1},
	}
}
