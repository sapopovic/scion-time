package client

import (
	"context"
	"net/netip"
	"time"
)

type CSPTPClientIP struct{}

func (c *CSPTPClientIP) measureClockOffset(ctx context.Context, localAddr, remoteAddr netip.Addr) (
	timestamp time.Time, offset time.Duration, err error) {
	return
}
