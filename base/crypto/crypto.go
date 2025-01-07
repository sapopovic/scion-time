package crypto

// Random numbers with a given upper bound and reservoir sampling using a
// cryptographically secure random number generator based on
// Daniel Lemire, Fast Random Integer Generation in an Interval
// ACM Transactions on Modeling and Computer Simulation 29 (1), 2019
// https://lemire.me/en/publication/arxiv1805/

import (
	"context"
	"crypto/rand"
	"encoding/binary"
	"math"
)

func randInt31(ctx context.Context, n int) (int, error) {
	if n < 2 {
		return 0, nil
	}
	if n > math.MaxInt32 {
		panic("invalid argument: n must not be greater than 2147483647")
	}
	t := uint32(-n) % uint32(n)
	b := make([]byte, 4)
	var x uint32
	for {
		n, err := rand.Read(b)
		if err != nil {
			return 0, err
		}
		if n != len(b) {
			panic("unexpected result from random number generator")
		}
		x = binary.LittleEndian.Uint32(b)
		if x > t {
			break
		}
		err = ctx.Err()
		if err != nil {
			return 0, err
		}
	}
	return int(x % uint32(n)), nil
}

func randInt63(ctx context.Context, n int) (int, error) {
	if n < 2 {
		return 0, nil
	}
	t := uint64(-n) % uint64(n)
	b := make([]byte, 8)
	var x uint64
	for {
		n, err := rand.Read(b)
		if err != nil {
			return 0, err
		}
		if n != len(b) {
			panic("unexpected result from random number generator")
		}
		x = binary.LittleEndian.Uint64(b)
		if x > t {
			break
		}
		err = ctx.Err()
		if err != nil {
			return 0, err
		}
	}
	return int(x % uint64(n)), nil
}

func RandIntn(ctx context.Context, n int) (int, error) {
	if n <= 0 {
		panic("invalid argument: n must be greater than 0")
	}
	if n <= math.MaxInt32 {
		return randInt31(ctx, n)
	}
	return randInt63(ctx, n)
}

func Sample(ctx context.Context, k, n int, pick func(dst, src int)) (int, error) {
	if k < 0 {
		panic("invalid argument: k must be non-negative")
	}
	if n < 0 {
		panic("invalid argument: n must be non-negative")
	}
	if n < k {
		k = n
	}
	for i := 0; i != k; i++ {
		pick(i, i)
	}
	for i := k; i != n; i++ {
		j, err := RandIntn(ctx, i+1)
		if err != nil {
			return 0, err
		}
		if j < k {
			pick(j, i)
		}
	}
	return k, nil
}
