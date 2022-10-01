package udp

import (
	"unsafe"

	"fmt"
	"net"
	"time"

	"github.com/scionproto/scion/go/lib/addr"
	"github.com/scionproto/scion/go/lib/snet"

	"golang.org/x/sys/unix"
)

var (
	errTimestampNotFound = fmt.Errorf("failed to read timestamp from out of band data")
	errUnexpectedData    = fmt.Errorf("failed to read out of band data")
)

type UDPAddr struct {
	IA   addr.IA
	Host *net.UDPAddr
}

func UDPAddrFromSnet(a *snet.UDPAddr) UDPAddr {
	return UDPAddr{a.IA, snet.CopyUDPAddr(a.Host)}
}

// Timestamp handling based on studying code from the following projects:
// - https://github.com/bsdphk/Ntimed, file udp.c
// - https://github.com/golang/go, package "golang.org/x/sys/unix"
// - https://github.com/google/gopacket, package "github.com/google/gopacket/pcapgo"
// - https://github.com/facebook/time, package "github.com/facebook/time/ntp/protocol/ntp"

func EnableTimestampingRaw(fd uintptr) {
	if so_timestampns != 0 {
		_ = unix.SetsockoptInt(int(fd), unix.SOL_SOCKET, so_timestampns, 1)
	} else if so_timestamp != 0 {
		_ = unix.SetsockoptInt(int(fd), unix.SOL_SOCKET, so_timestamp, 1)
	}
}

func EnableTimestamping(conn *net.UDPConn) {
	sconn, err := conn.SyscallConn()
	if err != nil {
		return
	}
	_ = sconn.Control(EnableTimestampingRaw)
}

func TimestampLen() int {
	return unix.CmsgSpace(int(unsafe.Sizeof(unix.Timespec{})))
}

func TimestampFromOOBData(oob []byte) (time.Time, error) {
	for unix.CmsgSpace(0) <= len(oob) {
		h := (*unix.Cmsghdr)(unsafe.Pointer(&oob[0]))
		if h.Len < unix.SizeofCmsghdr || uint64(h.Len) > uint64(len(oob)) {
			return time.Time{}, errUnexpectedData
		}
		if h.Level == unix.SOL_SOCKET {
			if scm_timestampns != 0 && h.Type == scm_timestampns {
				if unix.CmsgSpace(int(unsafe.Sizeof(unix.Timespec{}))) != int(h.Len) {
					return time.Time{}, errUnexpectedData
				}
				ts := (*unix.Timespec)(unsafe.Pointer(&oob[unix.CmsgSpace(0)]))
				return time.Unix(ts.Unix()), nil
			} else if scm_timestamp != 0 && h.Type == scm_timestamp {
				if unix.CmsgSpace(int(unsafe.Sizeof(unix.Timeval{}))) != int(h.Len) {
					return time.Time{}, errUnexpectedData
				}
				ts := (*unix.Timeval)(unsafe.Pointer(&oob[unix.CmsgSpace(0)]))
				return time.Unix(ts.Unix()), nil
			}
		}
		oob = oob[unix.CmsgSpace(int(h.Len))-unix.CmsgSpace(0):]
	}
	return time.Time{}, errTimestampNotFound
}
