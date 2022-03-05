package core

import (
	"context"
	"log"
	"time"

	"github.com/scionproto/scion/go/lib/addr"
	"github.com/scionproto/scion/go/lib/daemon"
	"github.com/scionproto/scion/go/lib/snet"
)

const patherLogPrefix = "[core/pather]"

const pathRefreshPeriod = 15 * time.Second

func StartPather(c daemon.Connector, peers []UDPAddr) (<-chan PathInfo, error) {
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

				paths := map[addr.IA][]snet.Path{}
				if peers == nil {
					//TODO: Implement peer lookup based on TRCs
					panic("not yet implemented: peer lookup based on TRCs")
				}
				for _, peer := range peers {
					if peer.IA.IsWildcard() {
						panic("unexpected peer IA: wildcard.")
					}
					ps, err := c.Paths(ctx, peer.IA, localIA, daemon.PathReqFlags{Refresh: true})
					if err != nil {
						log.Printf("%s Failed to look up peer paths: %v", patherLogPrefix, err)
					}
					for _, p := range ps {
						paths[p.Destination()] = append(paths[p.Destination()], p)
					}
				}
				log.Printf("%s Reachable peer ASes:", patherLogPrefix)
				for peerIA := range paths {
					log.Printf("%s %v", patherLogPrefix, peerIA)
					for _, p := range paths[peerIA] {
						log.Printf("%s \t%v", patherLogPrefix, p)
					}
				}

				pathInfos <- PathInfo{
					LocalIA: localIA,
					Paths:   paths,
				}
			}
		}
	}()

	return pathInfos, nil
}
