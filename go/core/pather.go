package core

import (
	"context"
	"log"
	"sync"
	"time"

	"github.com/scionproto/scion/pkg/addr"
	"github.com/scionproto/scion/pkg/daemon"
	"github.com/scionproto/scion/pkg/snet"
)

const patherLogPrefix = "[core/pather]"

const pathRefreshPeriod = 15 * time.Second

type Pather struct {
	mu      sync.Mutex
	localIA addr.IA
	paths   map[addr.IA][]snet.Path
}

func (p *Pather) LocalIA() addr.IA {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.localIA
}

func (p *Pather) Paths(ia addr.IA) []snet.Path {
	p.mu.Lock()
	defer p.mu.Unlock()
	paths, ok := p.paths[ia]
	if !ok {
		return nil
	}
	return append(make([]snet.Path, 0, len(paths)), paths...)
}

func update(ctx context.Context, p *Pather, c daemon.Connector, dstIAs []addr.IA) {
	log.Printf("%s Looking up peer paths", patherLogPrefix)

	localIA, err := c.LocalIA(ctx)
	if err != nil {
		log.Printf("%s Failed to look up local IA: %v", patherLogPrefix, err)
	}

	paths := map[addr.IA][]snet.Path{}
	for _, dstIA := range dstIAs {
		if dstIA.IsWildcard() {
			panic("unexpected destination IA: wildcard.")
		}
		ps, err := c.Paths(ctx, dstIA, localIA, daemon.PathReqFlags{Refresh: true})
		if err != nil {
			log.Printf("%s Failed to look up peer paths: %v", patherLogPrefix, err)
		}
		paths[dstIA] = append(paths[dstIA], ps...)
	}
	for peerIA := range paths {
		for _, p := range paths[peerIA] {
			log.Printf("%s %v:%v", peerIA, patherLogPrefix, p)
		}
	}

	p.mu.Lock()
	p.localIA = localIA
	p.paths = paths
	p.mu.Unlock()
}

func StartPather(c daemon.Connector, dstIAs []addr.IA) *Pather {
	p := &Pather{}
	go func(p *Pather, c daemon.Connector, dstIAs []addr.IA) {
		ctx := context.Background()
		ticker := time.NewTicker(pathRefreshPeriod)
		for {
			select {
			case <-ticker.C:
				update(ctx, p, c, dstIAs)
			}
		}
	}(p, c, dstIAs)
	return p
}
