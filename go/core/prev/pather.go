package core

import (
	"context"
	"io/ioutil"
	"log"
	"net"
	"time"

	"github.com/scionproto/scion/go/lib/addr"
	"github.com/scionproto/scion/go/lib/daemon"
	"github.com/scionproto/scion/go/lib/snet"
)

type PathInfo struct {
	LocalIA      addr.IA
	PeerASes     map[addr.IA][]snet.Path
	LocalTSHosts []*net.UDPAddr
}

var patherLog = log.New(ioutil.Discard, "[tsp/pather] ", log.LstdFlags)

func StartPather(c daemon.Connector, ctx context.Context, peersIAs []addr.IA) (<-chan PathInfo, error) {
	if peersIAs == nil {
		peersIAs = []addr.IA{{I: 0, A: 0}}
	} else {
		for _, peerIA := range peersIAs {
			if peerIA.IsWildcard() {
				panic("Unexpected peer IA: wildcard.")
			}
		}
	}
	localIA, err := c.LocalIA(ctx)
	if err != nil {
		return nil, err
	}

	pathInfos := make(chan PathInfo)

	go func() {
		ticker := time.NewTicker(15 * time.Second)

		for {
			select {
			case <-ticker.C:
				patherLog.Printf("Looking up TSP broadcast paths\n")

				peerASes := make(map[addr.IA][]snet.Path)
				for _, peerIA := range peersIAs {
					if !peerIA.IsWildcard() {
						peerASes[peerIA] = nil
					}
				}
				for _, peerIA := range peersIAs {
					peerPaths, err := c.Paths(ctx,
						peerIA, localIA, daemon.PathReqFlags{Refresh: true})
					if err != nil {
						patherLog.Printf("Failed to lookup peer paths: %v\n", err)
					}
					for _, p := range peerPaths {
						peerASes[p.Destination()] = append(peerASes[p.Destination()], p)
					}
				}

				patherLog.Printf("Reachable peer ASes:\n")
				for peerAS := range peerASes {
					patherLog.Printf("%v", peerAS)
					for _, p := range peerASes[peerAS] {
						patherLog.Printf("\t%v\n", p)
					}
				}

				localSvcInfo, err := c.SVCInfo(ctx, []addr.HostSVC{addr.SvcTS})
				if err != nil {
					patherLog.Printf("Failed to lookup local TS service info: %v\n", err)
				}
				var localTSHosts []*net.UDPAddr
				localSvcAddr, ok := localSvcInfo[addr.SvcTS]
				if ok {
					localSvcUdpAddr, err := net.ResolveUDPAddr("udp", localSvcAddr)
					if err != nil {
						patherLog.Printf("Failed to resolve local TS service addr: %v\n", err)
					}
					localTSHosts = append(localTSHosts, localSvcUdpAddr)
				}

				patherLog.Printf("Reachable local time services:\n")
				for _, localTSHost := range localTSHosts {
					patherLog.Printf("\t%v\n", localTSHost)
				}

				pathInfos <- PathInfo{
					LocalIA:      localIA,
					PeerASes:     peerASes,
					LocalTSHosts: localTSHosts,
				}
			}
		}
	}()

	return pathInfos, nil
}
