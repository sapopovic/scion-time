package core

import (
	"context"
	"sync"
	"time"

	"go.uber.org/zap"

	"github.com/scionproto/scion/pkg/addr"
	"github.com/scionproto/scion/pkg/daemon"
	"github.com/scionproto/scion/pkg/snet"
)

const pathRefreshPeriod = 15 * time.Second

type Pather struct {
	log     *zap.Logger
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
	localIA, err := c.LocalIA(ctx)
	if err != nil {
		p.log.Info("failed to look up local IA", zap.Error(err))
		return
	}

	paths := map[addr.IA][]snet.Path{}
	for _, dstIA := range dstIAs {
		if dstIA.IsWildcard() {
			panic("unexpected destination IA: wildcard.")
		}
		ps, err := c.Paths(ctx, dstIA, localIA, daemon.PathReqFlags{Refresh: true})
		if err != nil {
			p.log.Info("failed to look up paths", zap.Stringer("to", dstIA), zap.Error(err))
		}
		paths[dstIA] = append(paths[dstIA], ps...)
	}

	p.mu.Lock()
	p.localIA = localIA
	p.paths = paths
	p.mu.Unlock()
}

func StartPather(log *zap.Logger, c daemon.Connector, dstIAs []addr.IA) *Pather {
	ctx := context.Background()
	p := &Pather{log: log}
	update(ctx, p, c, dstIAs)
	go func(ctx context.Context, p *Pather, c daemon.Connector, dstIAs []addr.IA) {
		ticker := time.NewTicker(pathRefreshPeriod)
		for range ticker.C {
			update(ctx, p, c, dstIAs)
		}
	}(ctx, p, c, dstIAs)
	return p
}
