package core

import (
	"context"

	"github.com/scionproto/scion/pkg/daemon"

	"go.uber.org/zap"
)

func newDaemonConnector(ctx context.Context, log *zap.Logger, daemonAddr string) daemon.Connector {
	s := &daemon.Service{
		Address: daemonAddr,
	}
	c, err := s.Connect(ctx)
	if err != nil {
		log.Fatal("failed to create demon connector", zap.Error(err))
	}
	return c
}
