package drkeyutil

import (
	"context"

	"github.com/scionproto/scion/pkg/daemon"
	"github.com/scionproto/scion/pkg/drkey"
)

type Fetcher struct {
	dc daemon.Connector
}

func (f *Fetcher) FetchSecretValue(ctx context.Context, meta drkey.SecretValueMeta) (drkey.SecretValue, error) {
	return FetchSecretValue(ctx, f.dc, meta)
}

func (f *Fetcher) FetchHostHostKey(ctx context.Context, meta drkey.HostHostMeta) (drkey.HostHostKey, error) {
	return f.dc.DRKeyGetHostHostKey(ctx, meta)
}

func NewFetcher(c daemon.Connector) *Fetcher {
	return &Fetcher{dc: c}
}
