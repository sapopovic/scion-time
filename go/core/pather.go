package core

import (
	"context"
	"log"
	"net"
	"time"

	"github.com/scionproto/scion/go/lib/addr"
	"github.com/scionproto/scion/go/lib/daemon"
	"github.com/scionproto/scion/go/lib/snet"
)

const patherLogPrefix = "[core/pather]"

type PathInfo struct {
	LocalIA     addr.IA
	PeerIAPaths map[addr.IA][]snet.Path
	LocalPeers  []*net.UDPAddr
}

const pathRefreshPeriod = 15 * time.Second

func StartPather(c daemon.Connector, peerIAs []addr.IA) (<-chan PathInfo, error) {
	pathInfos := make(chan PathInfo)

	go func() {
		ctx := context.Background()
		ticker := time.NewTicker(pathRefreshPeriod)
		for {
			select {
			case <-ticker.C:
				log.Printf("%s Looking up peer paths", patherLogPrefix)

				localIA, err := c.LocalIA(ctx)
				if err != nil {
					log.Printf("%s Failed to look up local IA: %v", patherLogPrefix, err)
				}

				peerIAPaths := map[addr.IA][]snet.Path{}
				if peerIAs == nil {
					//TODO: Implement peerIA lookup based on TRCs				
					panic("Not yet implemented: peer IA lookup based on TRCs")
				}
				for _, peerIA := range peerIAs {
					if peerIA.IsWildcard() {
						panic("Unexpected peer IA: wildcard.")
					}
				}
				for _, peerIA := range peerIAs {
					ps, err := c.Paths(ctx, peerIA, localIA, daemon.PathReqFlags{Refresh: true})
					if err != nil {
						log.Printf("%s Failed to look up peer paths: %v", patherLogPrefix, err)
					}
					for _, p := range ps {
						peerIAPaths[p.Destination()] = append(peerIAPaths[p.Destination()], p)
					}
				}
				log.Printf("%s Reachable peer ASes:", patherLogPrefix)
				for peerIA := range peerIAPaths {
					log.Printf("%s %v", patherLogPrefix, peerIA)
					for _, p := range peerIAPaths[peerIA] {
						log.Printf("%s \t%v", patherLogPrefix, p)
					}
				}

				var localPeers []*net.UDPAddr
				// TODO: Implement local peer lookup
				// localSvcInfo, err := c.SVCInfo(ctx, []addr.HostSVC{addr.SvcTS})
				// if err != nil {
				// 	log.Printf("%s Failed to lookup local TS service info: %v", patherLogPrefix, err)
				// }
				// localSvcAddr, ok := localSvcInfo[addr.SvcTS]
				// if ok {
				// 	localSvcUdpAddr, err := net.ResolveUDPAddr("udp", localSvcAddr)
				// 	if err != nil {
				// 		log.Printf("%s Failed to resolve local TS service addr: %v", patherLogPrefix, err)
				// 	}
				// 	localPeers = append(localPeers, localSvcUdpAddr)
				// }
				log.Printf("%s Reachable local peers:", patherLogPrefix)
				for _, localPeer := range localPeers {
					log.Printf("%s \t%v", patherLogPrefix, localPeer)
				}

				pathInfos <- PathInfo{
					LocalIA:     localIA,
					PeerIAPaths: peerIAPaths,
					LocalPeers:  localPeers,
				}
			}
		}
	}()

	return pathInfos, nil
}
