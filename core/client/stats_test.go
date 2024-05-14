package client

// Based on an OpenAI GPT-4o interaction

import (
	"testing"
)

func TestMedian(t *testing.T) {
	tests := []struct {
		input    []float64
		expected float64
	}{
		// Test with an odd number of elements
		{input: []float64{1, 3, 2}, expected: 2},
		{input: []float64{7, 1, 3, 2, 5}, expected: 3},
		{input: []float64{10}, expected: 10},

		// Test with an even number of elements
		{input: []float64{1, 2, 3, 4}, expected: 2.5},
		{input: []float64{5, 1, 3, 2}, expected: 2.5},

		// Test with repeated elements
		{input: []float64{1, 2, 2, 3, 3}, expected: 2},
		{input: []float64{1, 1, 1, 1}, expected: 1},

		// Test with sorted input
		{input: []float64{1, 2, 3}, expected: 2},
		{input: []float64{1, 2, 3, 4}, expected: 2.5},
	}

	for _, test := range tests {
		t.Run("", func(t *testing.T) {
			result := median(test.input)
			if result != test.expected {
				t.Errorf("median(%v) = %v; expected %v", test.input, result, test.expected)
			}
		})
	}

	t.Run("NilSlice", func(t *testing.T) {
		defer func() {
			if r := recover(); r == nil {
				t.Errorf("median of nil slice did not panic")
			}
		}()
		median(nil)
	})

	t.Run("EmptySlice", func(t *testing.T) {
		defer func() {
			if r := recover(); r == nil {
				t.Errorf("median of empty slice did not panic")
			}
		}()
		median([]float64{})
	})
}
