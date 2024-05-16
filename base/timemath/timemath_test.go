package timemath_test

// Based on an OpenAI GPT-4 and GPT-4o interaction

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
		{time.Second, 1.0},
		{0, 0.0},
		{-time.Second, -1.0},
		{-1500 * time.Millisecond, -1.5},
	}

	for _, tt := range tests {
		got := tt.duration.Seconds()
		if got != tt.want {
			t.Errorf("(%v).Seconds() = %v, want %v", tt.duration, got, tt.want)
		}
	}
}


func TestSgn(t *testing.T) {
	tests := []struct {
		duration time.Duration
		want     int
	}{
		{0, 0},
		{-time.Second, -1},
		{time.Second, 1},
		{math.MinInt64, -1},
		{math.MaxInt64, 1},
	}

	for _, tt := range tests {
		got := timemath.Sgn(tt.duration)
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
		{0, 0},
		{-time.Second, time.Second},
		{time.Second, -time.Second},
		{math.MinInt64, math.MaxInt64},
		{math.MaxInt64, -math.MaxInt64},
	}

	for _, tt := range tests {
		got := timemath.Inv(tt.duration)
		if got != tt.want {
			t.Errorf("timemath.Inv(%v) = %v, want %v", tt.duration, got, tt.want)
		}
	}
}

func TestMedian(t *testing.T) {
	tests := []struct {
		name      string
		input     []time.Duration
		want      time.Duration
		wantPanic bool
	}{
		{
			name:      "Nil slice",
			input:     nil,
			wantPanic: true,
		},
		{
			name:      "Empty slice",
			input:     []time.Duration{},
			wantPanic: true,
		},
		{
			name:  "Single element",
			input: []time.Duration{42},
			want:  42,
		},
		{
			name:  "Two elements",
			input: []time.Duration{10, 20},
			want:  15,
		},
		{
			name:  "Three elements",
			input: []time.Duration{30, 10, 20},
			want:  20,
		},
		{
			name:  "Four elements",
			input: []time.Duration{40, 10, 30, 20},
			want:  25,
		},
		{
			name:  "Five elements",
			input: []time.Duration{50, 40, 30, 20, 10},
			want:  30,
		},
		{
			name:  "Six elements",
			input: []time.Duration{60, 50, 40, 30, 20, 10},
			want:  35,
		},
		{
			name:  "Seven elements",
			input: []time.Duration{70, 60, 50, 40, 30, 20, 10},
			want:  40,
		},
		{
			name:  "Eight elements",
			input: []time.Duration{80, 70, 60, 50, 40, 30, 20, 10},
			want:  45,
		},
		{
			name:  "Duplicate values",
			input: []time.Duration{10, 20, 20, 30, 30, 40},
			want:  25,
		},
		{
			name:  "Negative values",
			input: []time.Duration{-10, -20, -30, -40, -50},
			want:  -30,
		},
		{
			name:  "Mixed positive and negative values",
			input: []time.Duration{-10, 20, -30, 40, -50, 60},
			want:  5,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.wantPanic {
				defer func() {
					if r := recover(); r == nil {
						t.Errorf("expected panic, got none")
					}
				}()
				_ = timemath.Median(tt.input)
			} else {
				got := timemath.Median(tt.input)
				if got != tt.want {
					t.Errorf("Median(%v) = %v, want %v", tt.input, got, tt.want)
				}
			}
		})
	}
}

func TestFaultTolerantMidpoint(t *testing.T) {
	tests := []struct {
		name      string
		input     []time.Duration
		want      time.Duration
		wantPanic bool
	}{
		{
			name:      "Nil slice",
			input:     nil,
			wantPanic: true,
		},
		{
			name:      "Empty slice",
			input:     []time.Duration{},
			wantPanic: true,
		},
		{
			name:  "Single element",
			input: []time.Duration{42},
			want:  42,
		},
		{
			name:  "Two elements",
			input: []time.Duration{10, 20},
			want:  15,
		},
		{
			name:  "Three elements",
			input: []time.Duration{30, 10, 20},
			want:  20,
		},
		{
			name:  "Four elements",
			input: []time.Duration{40, 10, 30, 20},
			want:  25,
		},
		{
			name:  "Five elements",
			input: []time.Duration{50, 40, 30, 20, 10},
			want:  30,
		},
		{
			name:  "Six elements",
			input: []time.Duration{60, 50, 40, 30, 20, 10},
			want:  35,
		},
		{
			name:  "Seven elements",
			input: []time.Duration{70, 60, 50, 40, 30, 20, 10},
			want:  40,
		},
		{
			name:  "Eight elements",
			input: []time.Duration{80, 70, 60, 50, 40, 30, 20, 10},
			want:  45,
		},
		{
			name:  "Duplicate values",
			input: []time.Duration{10, 20, 20, 30, 30, 40},
			want:  25,
		},
		{
			name:  "Negative values",
			input: []time.Duration{-10, -20, -30, -40, -50},
			want:  -30,
		},
		{
			name:  "Mixed positive and negative values",
			input: []time.Duration{-10, 20, -30, 40, -50, 60},
			want:  5,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.wantPanic {
				defer func() {
					if r := recover(); r == nil {
						t.Errorf("expected panic, got none")
					}
				}()
				_ = timemath.FaultTolerantMidpoint(tt.input)
			} else {
				got := timemath.FaultTolerantMidpoint(tt.input)
				if got != tt.want {
					t.Errorf("FaultTolerantMidpoint(%v) = %v, want %v", tt.input, got, tt.want)
				}
			}
		})
	}
}
