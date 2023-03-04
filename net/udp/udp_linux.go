package udp

import (
	"unsafe"

	"net"
	"time"

	"golang.org/x/sys/unix"
)

func EnableRxTimestamps(conn *net.UDPConn) error {
	sconn, err := conn.SyscallConn()
	if err != nil {
		return err
	}
	var res struct {
		err error
	}
	err = sconn.Control(func(fd uintptr) {
		res.err = unix.SetsockoptInt(int(fd), unix.SOL_SOCKET, unix.SO_TIMESTAMPNS, 1)
	})
	if err != nil {
		return err
	}
	return res.err
}

func TimestampFromOOBData(oob []byte) (time.Time, error) {
	for unix.CmsgSpace(0) <= len(oob) {
		h := (*unix.Cmsghdr)(unsafe.Pointer(&oob[0]))
		if h.Len < unix.SizeofCmsghdr || h.Len > uint64(len(oob)) {
			return time.Time{}, errUnexpectedData
		}
		if h.Level == unix.SOL_SOCKET {
			if h.Type == unix.SO_TIMESTAMPING_NEW {
				if h.Len != uint64(unix.CmsgSpace(3*16)) {
					return time.Time{}, errUnexpectedData
				}
				sec := *(*int64)(unsafe.Pointer(&oob[unix.CmsgSpace(0)]))
				nsec := *(*int64)(unsafe.Pointer(&oob[unix.CmsgSpace(8)]))
				return time.Unix(sec, nsec), nil
			} else if h.Type == unix.SCM_TIMESTAMPNS {
				if h.Len != uint64(unix.CmsgSpace(int(unsafe.Sizeof(unix.Timespec{})))) {
					return time.Time{}, errUnexpectedData
				}
				ts := (*unix.Timespec)(unsafe.Pointer(&oob[unix.CmsgSpace(0)]))
				return time.Unix(ts.Unix()), nil
			}
		}
		oob = oob[unix.CmsgSpace(int(h.Len))-unix.CmsgSpace(0):]
	}
	return time.Time{}, errTimestampNotFound
}

func EnableTimestamping(conn *net.UDPConn, iface string) error {
	sconn, err := conn.SyscallConn()
	if err != nil {
		return err
	}
	var res struct {
		err error
	}
	err = sconn.Control(func(fd uintptr) {
		res.err = unix.SetsockoptInt(int(fd), unix.SOL_SOCKET, unix.SO_TIMESTAMPING_NEW,
			unix.SOF_TIMESTAMPING_SOFTWARE|
				unix.SOF_TIMESTAMPING_RX_SOFTWARE|
				unix.SOF_TIMESTAMPING_TX_SOFTWARE|
				unix.SOF_TIMESTAMPING_OPT_TSONLY|
				unix.SOF_TIMESTAMPING_OPT_ID)
	})
	if err != nil {
		return err
	}
	return res.err
}

func timestampFromOOBData(oob []byte) (time.Time, uint32, error) {
	var tsSet, idSet bool
	var ts time.Time
	var id uint32
	for unix.CmsgSpace(0) <= len(oob) {
		h := (*unix.Cmsghdr)(unsafe.Pointer(&oob[0]))
		if h.Len < unix.SizeofCmsghdr || h.Len > uint64(len(oob)) {
			return time.Time{}, 0, errUnexpectedData
		}
		if h.Level == unix.SOL_SOCKET {
			if h.Type == unix.SO_TIMESTAMPING_NEW {
				if h.Len != uint64(unix.CmsgSpace(3*16)) {
					return time.Time{}, 0, errUnexpectedData
				}
				sec := *(*int64)(unsafe.Pointer(&oob[unix.CmsgSpace(0)]))
				nsec := *(*int64)(unsafe.Pointer(&oob[unix.CmsgSpace(8)]))
				ts = time.Unix(sec, nsec)
				tsSet = true
			}
		} else if h.Level == unix.SOL_IP && h.Type == unix.IP_RECVERR ||
			h.Level == unix.SOL_IPV6 && h.Type == unix.IPV6_RECVERR {
			if h.Len < uint64(unix.CmsgSpace(int(unsafe.Sizeof(unix.SockExtendedErr{})))) {
				return time.Time{}, 0, errUnexpectedData
			}
			seerr := *(*unix.SockExtendedErr)(unsafe.Pointer(&oob[unix.CmsgSpace(0)]))
			if seerr.Errno != uint32(unix.ENOMSG) {
				return time.Time{}, 0, errUnexpectedData
			}
			if seerr.Origin != unix.SO_EE_ORIGIN_TIMESTAMPING {
				return time.Time{}, 0, errUnexpectedData
			}
			id = seerr.Data
			idSet = true
		}
		oob = oob[unix.CmsgSpace(int(h.Len))-unix.CmsgSpace(0):]
	}
	if !tsSet || !idSet {
		return time.Time{}, 0, errTimestampNotFound
	}
	return ts, id, nil
}

func ReadTXTimestamp(conn *net.UDPConn) (time.Time, uint32, error) {
	sconn, err := conn.SyscallConn()
	if err != nil {
		return time.Time{}, 0, err
	}
	var res struct {
		ts  time.Time
		id  uint32
		err error
	}
	err = sconn.Read(func(fd uintptr) bool {
		pollFds := []unix.PollFd{
			{Fd: int32(fd), Events: unix.POLLPRI},
		}
		var n int
		for {
			n, err = unix.Poll(pollFds, 1 /* timeout */)
			if err == unix.EINTR {
				continue
			}
			break
		}
		if err != nil {
			res.err = err
			return true
		}
		if n != len(pollFds) {
			res.err = errTimestampNotFound
			return true
		}
		buf := make([]byte, 0)
		oob := make([]byte, 128)
		var oobn, flags int
		var srcAddr unix.Sockaddr
		for {
			n, oobn, flags, srcAddr, err = unix.Recvmsg(int(fd), buf, oob, unix.MSG_ERRQUEUE)
			if err == unix.EINTR {
				continue
			}
			break
		}
		if err != nil {
			res.err = err
			return true
		}
		if n != 0 {
			res.err = errUnexpectedData
			return true
		}
		if flags != unix.MSG_ERRQUEUE {
			res.err = errUnexpectedData
			return true
		}
		if srcAddr != nil {
			res.err = errUnexpectedData
			return true
		}
		res.ts, res.id, res.err = timestampFromOOBData(oob[:oobn])
		return true
	})
	if err != nil {
		return time.Time{}, 0, err
	}
	return res.ts, res.id, res.err
}
