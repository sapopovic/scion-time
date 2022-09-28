package core

import (
	"context"
	"time"
)

type measurement struct {
	off time.Duration
	err error
}

func collectMeasurements(ctx context.Context, off []time.Duration, ms chan measurement, n int) int {
	i := 0
	j := 0
loop:
	for i != n {
		select {
		case m := <-ms:
			if m.err == nil {
				if j != len(off) {
					off[j] = m.off
					j++
				}
			}
			i++
		case <-ctx.Done():
			break loop
		}
	}
	go func(n int) { // drain channel
		for n != 0 {
			<-ms
			n--
		}
	}(n - i)
	return j
}
