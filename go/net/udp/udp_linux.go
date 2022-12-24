package udp

import (
	"unsafe"

	"fmt"
	"log"
	"net"
	"time"

	"golang.org/x/sys/unix"
)

const (
	scm_timestamp   = unix.SCM_TIMESTAMP
	scm_timestampns = unix.SCM_TIMESTAMPNS
	so_timestamp    = unix.SO_TIMESTAMP
	so_timestampns  = unix.SO_TIMESTAMPNS
)

var so_timestamping int = unix.SO_TIMESTAMPING_NEW

func EnableTimestamping(conn *net.UDPConn) {
	sconn, err := conn.SyscallConn()
	if err != nil {
		return
	}
	_ = sconn.Control(func (fd uintptr) {
		val := unix.SOF_TIMESTAMPING_SOFTWARE |
			unix.SOF_TIMESTAMPING_RX_SOFTWARE |
			unix.SOF_TIMESTAMPING_TX_SOFTWARE |
			unix.SOF_TIMESTAMPING_OPT_TSONLY |
			unix.SOF_TIMESTAMPING_OPT_ID
		_ = unix.SetsockoptInt(int(fd), unix.SOL_SOCKET, so_timestamping, val)
	})
}

func timestampFromOOBData(oob []byte) (time.Time, error) {
	for unix.CmsgSpace(0) <= len(oob) {
		h := (*unix.Cmsghdr)(unsafe.Pointer(&oob[0]))
		if h.Len < unix.SizeofCmsghdr || uint64(h.Len) > uint64(len(oob)) {
			return time.Time{}, errUnexpectedData
		}
		if h.Level == unix.SOL_SOCKET {
			if h.Type == unix.SO_TIMESTAMPING || h.Type == unix.SO_TIMESTAMPING_NEW {
				if unix.CmsgSpace(3*16) != int(h.Len) {
					return time.Time{}, errUnexpectedData
				}
				sec := *(*int64)(unsafe.Pointer(&oob[unix.CmsgSpace(0)]))
				nsec := *(*int64)(unsafe.Pointer(&oob[unix.CmsgSpace(8)]))
				return time.Unix(sec, nsec), nil
			}
		}
		oob = oob[unix.CmsgSpace(int(h.Len))-unix.CmsgSpace(0):]
	}
	return time.Time{}, errTimestampNotFound
}

func ReceiveTXTimestamp(conn *net.UDPConn) (time.Time, int32, error) {
	sconn, err := conn.SyscallConn()
	if err != nil {
		return time.Time{}, 0, err
	}
	var t time.Time
	_ = sconn.Read(func (fd uintptr) (bool) {
		pollFds := []unix.PollFd{
			{Fd: int32(fd), Events: unix.POLLPRI},
		}
		for {
			_, err := unix.Poll(pollFds, -1 /* timeout */)
			if err == unix.EINTR {
				continue
			}
			if err != nil {
				panic(fmt.Sprintf("%s unix.Poll failed: %v", "udp_linux", err))
			}
			break
		}
		buf := make([]byte, 1)
		oob := make([]byte, 128)
    n, oobn, flags, srcAddr, err := unix.Recvmsg(int(fd), buf, oob, unix.MSG_ERRQUEUE)
    if err !=  nil {
    	return true
    }
    if n != 0 {
    	err = errUnexpectedData
    	return true
    }
    if flags != unix.MSG_ERRQUEUE {
    	err = errUnexpectedData
    	return true
    }
    if srcAddr != nil {
    	err = errUnexpectedData
    	return true
    }
    log.Print("@@@", oobn, oob)
    t, err = timestampFromOOBData(oob[:oobn])
		return true
	})
	if err != nil {
		return time.Time{}, 0, err
	}
	return t, 0, err
}