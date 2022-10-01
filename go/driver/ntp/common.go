package ntp

import (
	"fmt"
	"time"
)

const (
	ntpLogPrefix = "[driver/ntp]"

	timeout = 1 * time.Second
)

var (
	errUnexpectedPacketFlags   = fmt.Errorf("failed to read packet: unexpected flags")
	errUnexpectedPacketPayload = fmt.Errorf("failed to read packet: unexpected payload")
)
