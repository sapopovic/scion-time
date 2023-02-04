package drkeyutil

import (
	"context"

	"github.com/scionproto/scion/pkg/daemon"
	"github.com/scionproto/scion/pkg/drkey"
)

type Fetcher struct {
	dc  daemon.Connector
	svs map[drkey.Protocol]drkey.SecretValue
}

func (f *Fetcher) FetchSecretValue(ctx context.Context, meta drkey.SecretValueMeta) (drkey.SecretValue, error) {
	var err error
	sv, ok := f.svs[meta.ProtoId]
	if !ok || !sv.Epoch.Contains(meta.Validity) {
		sv, err = FetchSecretValue(ctx, f.dc, meta)
		if err == nil {
			f.svs[meta.ProtoId] = sv
		}
	}
	return sv, err
}

func (f *Fetcher) FetchHostHostKey(ctx context.Context, meta drkey.HostHostMeta) (drkey.HostHostKey, error) {
	return f.dc.DRKeyGetHostHostKey(ctx, meta)
}

func NewFetcher(c daemon.Connector) *Fetcher {
	return &Fetcher{
		dc:  c,
		svs: make(map[drkey.Protocol]drkey.SecretValue),
	}
}
