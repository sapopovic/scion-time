package udp

import (
	"unsafe"

	"fmt"
	"net"
	"time"

	"golang.org/x/sys/unix"

	fbntp "github.com/facebook/time/ntp/protocol/ntp"
)

var (
	errTimestampNotFound = fmt.Errorf("failed to read timestamp from out of band data")
	errUnexpectedData    = fmt.Errorf("failed to read out of band data")
)

func EnableTimestamping(conn *net.UDPConn) error {
	return fbntp.EnableKernelTimestampsSocket(conn)
}

func TimestampOutOfBandDataLen() int {
	return unix.CmsgSpace(int(unsafe.Sizeof(unix.Timespec{})))
}

func TimeFromOutOfBandData(oob []byte) (time.Time, error) {
	for unix.CmsgSpace(0) <= len(oob) {
		h := (*unix.Cmsghdr)(unsafe.Pointer(&oob[0]))
		if h.Len < unix.SizeofCmsghdr || uint64(h.Len) > uint64(len(oob)) {
			return time.Time{}, errUnexpectedData
		}
		if h.Level == unix.SOL_SOCKET {
			if h.Type == unix.SO_TIMESTAMPNS {
				if unix.CmsgSpace(int(unsafe.Sizeof(unix.Timespec{}))) != int(h.Len) {
					return time.Time{}, errUnexpectedData
				}
				ts := (*unix.Timespec)(unsafe.Pointer(&oob[unix.CmsgSpace(0)]))
				return time.Unix(int64(ts.Sec), int64(ts.Nsec)), nil
			} else if h.Type == unix.SO_TIMESTAMP {
				if unix.CmsgSpace(int(unsafe.Sizeof(unix.Timeval{}))) != int(h.Len) {
					return time.Time{}, errUnexpectedData
				}
				ts := (*unix.Timeval)(unsafe.Pointer(&oob[unix.CmsgSpace(0)]))
				return time.Unix(int64(ts.Sec), int64(ts.Usec) * 1000), nil
			}
		}
		oob = oob[unix.CmsgSpace(int(h.Len)) - unix.CmsgSpace(0):]
	}
	return time.Time{}, errTimestampNotFound
}
