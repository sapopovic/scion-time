package udp

import (
	"errors"
	"net"

	"github.com/scionproto/scion/pkg/addr"
	"github.com/scionproto/scion/pkg/snet"

	"golang.org/x/sys/unix"
)

const (
	HdrLen = 8
)

var (
	errTimestampNotFound = errors.New("failed to read timestamp from out of band data")
	errUnexpectedData    = errors.New("failed to read out of band data")
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

func TimestampLen() int {
	return unix.CmsgSpace(3 * 16)
}
