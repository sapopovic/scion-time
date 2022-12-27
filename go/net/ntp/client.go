package ntp

import (
	"errors"
	"time"
)

var errUnexpectedResponse = errors.New("unexpected response structure")

func ValidateMetadata(resp *Packet) error {
	// Based on Ntimed by Poul-Henning Kamp, https://github.com/bsdphk/Ntimed

	if resp.LeapIndicator() == LeapIndicatorUnknown {
		return errUnexpectedResponse
	}
	if resp.Version() != 3 && resp.Version() != 4 {
		return errUnexpectedResponse
	}
	if resp.Mode() != ModeServer {
		return errUnexpectedResponse
	}
	if resp.Stratum == 0 || resp.Stratum > 15 {
		return errUnexpectedResponse
	}
	return nil
}

func ValidateTimestamps(t0, t1, t2, t3 time.Time) error {
	if t3.Sub(t0) < 0 {
		panic("non monotonic local clock")
	}
	if t2.Sub(t1) < 0 {
		return errUnexpectedResponse
	}
	return nil
}
