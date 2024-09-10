package unixutil_test

// Based on an Claude AI interaction

import (
	"testing"

	"example.com/scion-time/base/unixutil"
)

func TestScaledPPMFromFreq(t *testing.T) {
	tests := []struct {
		name     string
		freq     float64
		expected int64
	}{
		{"Zero frequency", 0, 0},
		{"Positive frequency", 1, 65536000000},
		{"Negative frequency", -1, -65536000000},
		{"Small positive frequency", 0.000001, 65536},
		{"Small negative frequency", -0.000001, -65536},
		{"Large positive frequency", 1000, 65536000000000},
		{"Large negative frequency", -1000, -65536000000000},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := unixutil.ScaledPPMFromFreq(tt.freq)
			if result != tt.expected {
				t.Errorf("ScaledPPMFromFreq(%f) = %d; want %d", tt.freq, result, tt.expected)
			}
		})
	}
}

func TestFreqFromScaledPPM(t *testing.T) {
	tests := []struct {
		name      string
		scaledPPM int64
		expected  float64
	}{
		{"Zero scaled PPM", 0, 0},
		{"Positive scaled PPM", 65536000000, 1},
		{"Negative scaled PPM", -65536000000, -1},
		{"Small positive scaled PPM", 65536, 0.000001},
		{"Small negative scaled PPM", -65536, -0.000001},
		{"Large positive scaled PPM", 65536000000000, 1000},
		{"Large negative scaled PPM", -65536000000000, -1000},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := unixutil.FreqFromScaledPPM(tt.scaledPPM)
			if result != tt.expected {
				t.Errorf("FreqFromScaledPPM(%d) = %f; want %f", tt.scaledPPM, result, tt.expected)
			}
		})
	}
}
