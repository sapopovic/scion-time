package ntp

import (
	"errors"
	"time"
)

var errUnexpectedResponse = errors.New("failed to validate response")

func ValidateResponse(resp *Packet, reqTransmitTime time.Time) error {
	respOriginTime := TimeFromTime64(resp.OriginTime)
	if respOriginTime.Sub(reqTransmitTime) != 0 {
		return errUnexpectedResponse
	}
	return nil
}
