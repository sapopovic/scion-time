package core

import (
	"net"

	"github.com/scionproto/scion/go/lib/addr"
	"github.com/scionproto/scion/go/lib/snet"
)

type UDPAddr struct {
	IA   addr.IA
	Host *net.UDPAddr
}

type PathInfo struct {
	LocalIA addr.IA
	Paths   map[addr.IA][]snet.Path
}
