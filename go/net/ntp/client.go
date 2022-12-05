package ntp

import (
	"errors"
)

var errUnexpectedResponse = errors.New("failed to validate response")

func ValidateResponse(resp *Packet) error {
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
	transmitTime := TimeFromTime64(resp.TransmitTime)
	receiveTime := TimeFromTime64(resp.ReceiveTime)
	if transmitTime.Sub(receiveTime) < 0 {
		return errUnexpectedResponse
	}
	referenceTime := TimeFromTime64(resp.ReferenceTime)
	d := transmitTime.Sub(referenceTime)
	if d.Nanoseconds() < -2 || d.Seconds() > 2048 {
		return errUnexpectedResponse
	}
	return nil
}
