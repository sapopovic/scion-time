package core

import (
	"log"
	"net"

	"github.com/scionproto/scion/go/lib/addr"
)

const ipServerLogPrefix = "[core/server_ip]"

func StartIPServer(localIA addr.IA, localHost *net.UDPAddr) error {
	log.Printf("%s Listening in %v on %v:%d via IP, NOT YET IMPLEMENTED", scionServerLogPrefix, localIA, localHost.IP, localHost.Port)
	return nil
}
