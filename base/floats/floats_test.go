package floats_test

// Based on an OpenAI GPT-4o interaction

import (
	"testing"

	"example.com/scion-time/base/floats"
)

func TestMedian(t *testing.T) {
	tests := []struct {
		name      string
		input     []float64
		want      float64
		wantPanic bool
	}{
		{
			name:      "Nil slice",
			input:     nil,
			wantPanic: true,
		},
		{
			name:      "Empty slice",
			input:     []float64{},
			wantPanic: true,
		},
		{
			name:  "Single element",
			input: []float64{42.0},
			want:  42.0,
		},
		{
			name:  "Two elements",
			input: []float64{1.0, 2.0},
			want:  1.5,
		},
		{
			name:  "Three elements",
			input: []float64{3.0, 1.0, 2.0},
			want:  2.0,
		},
		{
			name:  "Four elements",
			input: []float64{4.0, 1.0, 3.0, 2.0},
			want:  2.5,
		},
		{
			name:  "Five elements",
			input: []float64{5.0, 4.0, 3.0, 2.0, 1.0},
			want:  3.0,
		},
		{
			name:  "Six elements",
			input: []float64{6.0, 5.0, 4.0, 3.0, 2.0, 1.0},
			want:  3.5,
		},
		{
			name:  "Seven elements",
			input: []float64{7.0, 6.0, 5.0, 4.0, 3.0, 2.0, 1.0},
			want:  4.0,
		},
		{
			name:  "Eight elements",
			input: []float64{8.0, 7.0, 6.0, 5.0, 4.0, 3.0, 2.0, 1.0},
			want:  4.5,
		},
		{
			name:  "Duplicate values",
			input: []float64{1.0, 2.0, 2.0, 3.0, 3.0, 4.0},
			want:  2.5,
		},
		{
			name:  "Negative values",
			input: []float64{-1.0, -2.0, -3.0, -4.0, -5.0},
			want:  -3.0,
		},
		{
			name:  "Mixed positive and negative values",
			input: []float64{-1.0, 2.0, -3.0, 4.0, -5.0, 6.0},
			want:  0.5,
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
				_ = floats.Median(tt.input)
			} else {
				got := floats.Median(tt.input)
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
		input     []float64
		want      float64
		wantPanic bool
	}{
		{
			name:      "Nil slice",
			input:     nil,
			wantPanic: true,
		},
		{
			name:      "Empty slice",
			input:     []float64{},
			wantPanic: true,
		},
		{
			name:  "Single element",
			input: []float64{42.0},
			want:  42.0,
		},
		{
			name:  "Two elements",
			input: []float64{1.0, 2.0},
			want:  1.5,
		},
		{
			name:  "Three elements",
			input: []float64{3.0, 1.0, 2.0},
			want:  2.0,
		},
		{
			name:  "Four elements",
			input: []float64{4.0, 1.0, 3.0, 2.0},
			want:  2.5,
		},
		{
			name:  "Five elements",
			input: []float64{5.0, 4.0, 3.0, 2.0, 1.0},
			want:  3.0,
		},
		{
			name:  "Six elements",
			input: []float64{6.0, 5.0, 4.0, 3.0, 2.0, 1.0},
			want:  3.5,
		},
		{
			name:  "Seven elements",
			input: []float64{7.0, 6.0, 5.0, 4.0, 3.0, 2.0, 1.0},
			want:  4.0,
		},
		{
			name:  "Eight elements",
			input: []float64{8.0, 7.0, 6.0, 5.0, 4.0, 3.0, 2.0, 1.0},
			want:  4.5,
		},
		{
			name:  "Duplicate values",
			input: []float64{1.0, 2.0, 2.0, 3.0, 3.0, 4.0},
			want:  2.5,
		},
		{
			name:  "Negative values",
			input: []float64{-1.0, -2.0, -3.0, -4.0, -5.0},
			want:  -3.0,
		},
		{
			name:  "Mixed positive and negative values",
			input: []float64{-1.0, 2.0, -3.0, 4.0, -5.0, 6.0},
			want:  0.5,
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
				_ = floats.FaultTolerantMidpoint(tt.input)
			} else {
				got := floats.FaultTolerantMidpoint(tt.input)
				if got != tt.want {
					t.Errorf("FaultTolerantMidpoint(%v) = %v, want %v", tt.input, got, tt.want)
				}
			}
		})
	}
}
