package core

import (
	"log"
	"net"
	"time"

	"github.com/scionproto/scion/go/lib/addr"
	"github.com/scionproto/scion/go/lib/snet"

	"example.com/scion-time/go/net/ntp"
)

const clientLogPrefix = "[core/client]"

const (
	numClient            = 8
	numCollector         = 126
	requestBufferCapcity = 256
)

type syncInfo struct {
	Source      snet.SCIONAddress
	ClockOffset time.Duration
}

type request struct {
	peer  addr.IA
	paths snet.Path
}

type measurementRequest struct {
	peer  addr.IA
	paths []snet.Path
}

type requestHandler struct {
	id        int
	localIA   addr.IA
	localHost addr.HostAddr
	localPort uint16
	requests  chan request
}

var (
	localHost           net.UDPAddr
	requestHandlers     chan *requestHandler
	measurementRequests chan measurementRequest
)

// func newPropagator(id int, packetConn snet.PacketConn,
// 	localIA addr.IA, localHost addr.HostAddr, localPort uint16) propagator {
// 	return propagator{
// 		id:                id,
// 		packetConn:        packetConn,
// 		localIA:           localIA,
// 		localHost:         localHost,
// 		localPort:         localPort,
// 		propagateRequests: make(chan propagateRequest),
// 	}
// }

func (h *requestHandler) start() {
	go func() {
		for {
			requestHandlers <- h
			log.Printf("%s [%d] Awaiting requests", clientLogPrefix, h.id)
			select {
			case r := <-h.requests:
				log.Printf("%s [%d] Received request %v", clientLogPrefix, h.id, r)
				// r.pkt.Source = snet.SCIONAddress{IA: p.localIA, Host: p.localHost}
				// udpPayload := r.pkt.Payload.(snet.UDPPayload)
				// udpPayload.SrcPort = p.localPort
				// err := p.packetConn.WriteTo(r.pkt, r.nextHop)
				// if err != nil {
				// 	log.Printf("%s [%d] Failed to write packet: %v", propagatorLogPrefix, p.id, err)
				// }
				log.Printf("%s [%d] Handled request", clientLogPrefix, h.id)
			}
		}
	}()
}

// func StartClient(s snet.PacketDispatcherService, ctx context.Context,
// 	localIA addr.IA, localHost *net.UDPAddr) error {
// 	propagators = make(chan *propagator, nPropagators)
// 	propagateRequests = make(chan propagateRequest, nPropagateRequests)

// 	for i := 0; i != cap(propagators); i++ {
// 		packetConn, localPort, err := s.Register(ctx, localIA, localHost, addr.SvcNone)
// 		if err != nil {
// 			// TODO: stop already started propagators
// 			return err
// 		}
// 		p := newPropagator(i, packetConn, localIA, addr.HostFromIP(localHost.IP), localPort)
// 		p.start()
// 	}

// 	go func() {
// 		for {
// 			select {
// 			case r := <-measurementRequests:
// 				log.Printf("%s Received measurement request %v", clientLogPrefix, r)
// 				if len(r.paths) != 0 {
// 					p := r.paths[rand.Intn(len(r.paths))]
// 					h := <-requestHandlers
// 					h.requests <- request{r.peer, p}
// 				}
// 				log.Printf("%s Dispatched request %v", propagatorLogPrefix, r)
// 			}
// 		}
// 	}()

// 	return nil
// }

func handleResponse(resp *ntp.Packet, rxt time.Time) {
	panic("not yet implemented")
}

func startMeasurementRound(chan<- syncInfo) {
	panic("not yet implemented")
}

func measureClockOffset(peer addr.IA, paths []snet.Path) {
	panic("not yet implemented")
	measurementRequests <- measurementRequest{peer, paths}
}

func stopMeasurementRound() {
	panic("not yet implemented")
}
