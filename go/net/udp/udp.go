package udp

import (
	"net"

	fbntp "github.com/facebook/time/ntp/protocol/ntp"
)

const TimestampControlMessageLen = fbntp.ControlHeaderSizeBytes

func EnableTimestamping(conn *net.UDPConn) error {
	return fbntp.EnableKernelTimestampsSocket(conn)
}
