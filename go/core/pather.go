package core

import (
	"context"
	"io"
	"log"
	"net"
	"os"
	"time"

	"github.com/scionproto/scion/go/lib/addr"
	"github.com/scionproto/scion/go/lib/daemon"
	"github.com/scionproto/scion/go/lib/snet"
)

type PathInfo struct {
	LocalIA     addr.IA
	PeerIAPaths map[addr.IA][]snet.Path
	LocalPeers  []*net.UDPAddr
}

const pathRefreshPeriod = 15 * time.Second

var logWriter, _ = os.Stderr, io.Discard
var patherLog = log.New(logWriter, "[ts/pather] ", log.LstdFlags)

func StartPather(c daemon.Connector, peerIAs []addr.IA) (<-chan PathInfo, error) {
	pathInfos := make(chan PathInfo)

	go func() {
		ctx := context.Background()
		ticker := time.NewTicker(pathRefreshPeriod)
		for {
			select {
			case <-ticker.C:
				patherLog.Printf("Looking up peer paths")

				localIA, err := c.LocalIA(ctx)
				if err != nil {
					patherLog.Printf("Failed to look up local IA: %v", err)
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
						patherLog.Printf("Failed to look up peer paths: %v", err)
					}
					for _, p := range ps {
						peerIAPaths[p.Destination()] = append(peerIAPaths[p.Destination()], p)
					}
				}
				patherLog.Printf("Reachable peer ASes:")
				for peerIA := range peerIAPaths {
					patherLog.Printf("%v", peerIA)
					for _, p := range peerIAPaths[peerIA] {
						patherLog.Printf("\t%v", p)
					}
				}

				var localPeers []*net.UDPAddr
				// TODO: Implement local peer lookup
				// localSvcInfo, err := c.SVCInfo(ctx, []addr.HostSVC{addr.SvcTS})
				// if err != nil {
				// 	patherLog.Printf("Failed to lookup local TS service info: %v", err)
				// }
				// localSvcAddr, ok := localSvcInfo[addr.SvcTS]
				// if ok {
				// 	localSvcUdpAddr, err := net.ResolveUDPAddr("udp", localSvcAddr)
				// 	if err != nil {
				// 		patherLog.Printf("Failed to resolve local TS service addr: %v", err)
				// 	}
				// 	localPeers = append(localPeers, localSvcUdpAddr)
				// }
				patherLog.Printf("Reachable local peers:")
				for _, localPeer := range localPeers {
					patherLog.Printf("\t%v", localPeer)
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
