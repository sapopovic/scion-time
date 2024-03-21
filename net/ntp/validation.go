package ntp

import (
	"log"

	"errors"
	"time"
)

var (
	errUnexpectedRequest  = errors.New("unexpected request structure")
	errUnexpectedResponse = errors.New("unexpected response structure")
)

func ValidateResponseMetadata(resp *Packet) error {
	// Based on Ntimed by Poul-Henning Kamp, https://github.com/bsdphk/Ntimed

	if resp.LeapIndicator() == LeapIndicatorUnknown {
log.Print("@0")
		return errUnexpectedResponse
	}
	if resp.Version() != 3 && resp.Version() != 4 {
log.Print("@1")
		return errUnexpectedResponse
	}
	if resp.Mode() != ModeServer {
log.Print("@2")
		return errUnexpectedResponse
	}
	if resp.Stratum == 0 || resp.Stratum > 15 {
log.Print("@3")
		return errUnexpectedResponse
	}
	return nil
}

func ValidateResponseTimestamps(t0, t1, t2, t3 time.Time) error {
	if t3.Sub(t0) < 0 {
		panic("unexpected local clock behavior")
	}
	if t2.Sub(t1) < 0 {
log.Print("@4")
		return errUnexpectedResponse
	}
	return nil
}

func ValidateRequest(req *Packet, srcPort uint16) error {
	li := req.LeapIndicator()
	if li != LeapIndicatorNoWarning && li != LeapIndicatorUnknown {
		return errUnexpectedRequest
	}
	vn := req.Version()
	if vn < VersionMin || VersionMax < vn {
		return errUnexpectedRequest
	}
	mode := req.Mode()
	if vn == 1 && mode != ModeReserved0 || vn != 1 && mode != ModeClient {
		return errUnexpectedRequest
	}
	return nil
}
