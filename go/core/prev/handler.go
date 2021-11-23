package core

import (
	"context"
	"log"
	"net"
	"os"
	"time"

	"github.com/scionproto/scion/go/lib/addr"
	"github.com/scionproto/scion/go/lib/common"
	"github.com/scionproto/scion/go/lib/snet"
)

type SyncInfo struct {
	Source      snet.SCIONAddress
	ClockOffset time.Duration
}

var handlerLog = log.New(os.Stderr, "[tsp/handler] ", log.LstdFlags)

func StartHandler(s snet.PacketDispatcherService, ctx context.Context,
	localIA addr.IA, localHost *net.UDPAddr) (<-chan SyncInfo, error) {
	conn, localPort, err := s.Register(ctx, localIA, localHost, addr.SvcTS)
	if err != nil {
		return nil, err
	}

	handlerLog.Printf("Listening in %v on %v:%d - %v\n",
		localIA, localHost.IP, localPort, addr.SvcTS)

	syncInfos := make(chan SyncInfo)

	go func() {
		for {
			var packet snet.Packet
			var lastHop net.UDPAddr
			err := conn.ReadFrom(&packet, &lastHop)
			if err != nil {
				handlerLog.Printf("Failed to read packet: %v\n", err)
				continue
			}
			payload, ok := packet.Payload.(snet.UDPPayload)
			if !ok {
				handlerLog.Printf("Failed to read packet payload: %v\n", common.TypeOf(packet.Payload))
				continue
			}

			clockOffset, err := time.ParseDuration(string(payload.Payload))
			if err != nil {
				handlerLog.Printf("Failed to decode packet: %v\n", err)
				continue
			}

			syncInfos <- SyncInfo{
				Source:      packet.Source,
				ClockOffset: clockOffset,
			}
		}
	}()

	return syncInfos, nil
}
