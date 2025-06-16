package scion

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/scionproto/scion/pkg/addr"
	"github.com/scionproto/scion/pkg/daemon"
	"github.com/scionproto/scion/pkg/snet"
)

const pathRefreshPeriod = 15 * time.Second

var DC daemon.Connector

type Pather struct {
	log     *slog.Logger
	mu      sync.Mutex
	localIA addr.IA
	paths   map[addr.IA][]snet.Path
}

func (p *Pather) LocalIA() addr.IA {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.localIA
}

func (p *Pather) Paths(dst addr.IA) []snet.Path {
	p.mu.Lock()
	defer p.mu.Unlock()
	paths, ok := p.paths[dst]
	if !ok {
		return nil
	}
	return append(make([]snet.Path, 0, len(paths)), paths...)
}

func (p *Pather) GetPathsToDest(ctx context.Context, dc daemon.Connector, dstIA addr.IA) ([]snet.Path, error) {
	localIA, err := dc.LocalIA(ctx)
	if err != nil {
		p.log.LogAttrs(ctx, slog.LevelInfo,
			"failed to look up local IA", slog.Any("error", err))
		return []snet.Path{}, err
	}

	paths := []snet.Path{}
	if dstIA.IsWildcard() {
		panic("unexpected destination IA: wildcard.")
	}
	ps, err := DC.Paths(ctx, dstIA, localIA, daemon.PathReqFlags{Refresh: true})
	if err != nil {
		p.log.LogAttrs(ctx, slog.LevelInfo,
			"failed to look up paths", slog.Any("to", dstIA), slog.Any("error", err))
	}
	paths = append(paths, ps...)

	// p.mu.Lock()
	// p.localIA = localIA
	return paths, nil
	// p.mu.Unlock()

}

func update(ctx context.Context, p *Pather, dc daemon.Connector, dstIAs []addr.IA) {
	localIA, err := dc.LocalIA(ctx)
	if err != nil {
		p.log.LogAttrs(ctx, slog.LevelInfo,
			"failed to look up local IA", slog.Any("error", err))
		return
	}

	paths := map[addr.IA][]snet.Path{}
	for _, dstIA := range dstIAs {
		if dstIA.IsWildcard() {
			panic("unexpected destination IA: wildcard.")
		}
		ps, err := dc.Paths(ctx, dstIA, localIA, daemon.PathReqFlags{Refresh: true})
		if err != nil {
			p.log.LogAttrs(ctx, slog.LevelInfo,
				"failed to look up paths", slog.Any("to", dstIA), slog.Any("error", err))
		}
		paths[dstIA] = append(paths[dstIA], ps...)
	}

	p.mu.Lock()
	p.localIA = localIA
	p.paths = paths
	p.mu.Unlock()
}

func StartPather(ctx context.Context, log *slog.Logger, daemonAddr string, dstIAs []addr.IA) *Pather {
	p := &Pather{log: log}
	dc := NewDaemonConnector(ctx, daemonAddr)
	DC = dc
	update(ctx, p, dc, dstIAs)
	go func(ctx context.Context, p *Pather, dc daemon.Connector, dstIAs []addr.IA) {
		ticker := time.NewTicker(pathRefreshPeriod)
		for range ticker.C {
			update(ctx, p, dc, dstIAs)
		}
	}(ctx, p, dc, dstIAs)
	return p
}
