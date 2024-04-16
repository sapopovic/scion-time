package timemath_test

// Based on an OpenAI GPT-4 interaction

import (
	"math"
	"testing"
	"time"

	"example.com/scion-time/base/timemath"
)

func TestDuration(t *testing.T) {
	tests := []struct {
		seconds float64
		want    time.Duration
	}{
		{1.5, 1500 * time.Millisecond},
		{1, time.Second},
		{0, 0},
		{-1, -time.Second},
		{-1.5, -1500 * time.Millisecond},
	}

	for _, tt := range tests {
		got := timemath.Duration(tt.seconds)
		if got != tt.want {
			t.Errorf("timemath.Duration(%v) = %v, want %v", tt.seconds, got, tt.want)
		}
	}
}

func TestSeconds(t *testing.T) {
	tests := []struct {
		duration time.Duration
		want     float64
	}{
		{1500 * time.Millisecond, 1.5},
		{time.Second, 1},
		{0, 0},
		{-time.Second, -1},
		{-1500 * time.Millisecond, -1.5},
	}

	for _, tt := range tests {
		got := timemath.Seconds(tt.duration)
		if got != tt.want {
			t.Errorf("timemath.Seconds(%v) = %v, want %v", tt.duration, got, tt.want)
		}
	}
}

func TestAbs(t *testing.T) {
	tests := []struct {
		duration time.Duration
		want     time.Duration
	}{
		{time.Second, time.Second},
		{-time.Second, time.Second},
		{0, 0},
	}

	for _, tt := range tests {
		got := timemath.Abs(tt.duration)
		if got != tt.want {
			t.Errorf("timemath.Abs(%v) = %v, want %v", tt.duration, got, tt.want)
		}
	}

	// Testing panic for timemath.Abs with math.MinInt64
	defer func() {
		if r := recover(); r == nil {
			t.Errorf("timemath.Abs(%v), did not panic", math.MinInt64)
		}
	}()
	timemath.Abs(math.MinInt64)
}

func TestSign(t *testing.T) {
	tests := []struct {
		duration time.Duration
		want     int
	}{
		{time.Second, 1},
		{-time.Second, -1},
		{0, 0},
	}

	for _, tt := range tests {
		got := timemath.Sign(tt.duration)
		if got != tt.want {
			t.Errorf("timemath.Sign(%v) = %v, want %v", tt.duration, got, tt.want)
		}
	}
}

func TestInv(t *testing.T) {
	tests := []struct {
		duration time.Duration
		want     time.Duration
	}{
		{time.Second, -time.Second},
		{-time.Second, time.Second},
		{0, 0},
	}

	for _, tt := range tests {
		got := timemath.Inv(tt.duration)
		if got != tt.want {
			t.Errorf("timemath.Inv(%v) = %v, want %v", tt.duration, got, tt.want)
		}
	}

	// Testing panic for Inv with math.MinInt64
	defer func() {
		if r := recover(); r == nil {
			t.Errorf("timemath.Inv(%v), did not panic", math.MinInt64)
		}
	}()
	timemath.Inv(math.MinInt64)
}
