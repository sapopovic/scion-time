package scion

import (
	"context"
	"errors"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/scionproto/scion/pkg/addr"
	"github.com/scionproto/scion/pkg/daemon"
	"github.com/scionproto/scion/pkg/drkey"
	"github.com/scionproto/scion/pkg/drkey/specific"
	cppb "github.com/scionproto/scion/pkg/proto/control_plane"
	dkpb "github.com/scionproto/scion/pkg/proto/drkey"
	"github.com/scionproto/scion/pkg/scrypto/cppki"
)

var (
	errNoCSAddress = errors.New("failed to look up control service address")
)

func FetchSecretValue(ctx context.Context, dc daemon.Connector, meta drkey.SecretValueMeta) (
	drkey.SecretValue, error) {
	svcs, err := dc.SVCInfo(ctx, nil)
	if err != nil {
		return drkey.SecretValue{}, err
	}
	cs := svcs[addr.SvcCS]
	if len(cs) == 0 {
		return drkey.SecretValue{}, errNoCSAddress
	}

	conn, err := grpc.DialContext(ctx, cs[0], grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return drkey.SecretValue{}, err
	}
	defer conn.Close()
	client := cppb.NewDRKeyIntraServiceClient(conn)

	rep, err := client.DRKeySecretValue(ctx, &cppb.DRKeySecretValueRequest{
		ValTime:    timestamppb.New(meta.Validity),
		ProtocolId: dkpb.Protocol(meta.ProtoId),
	})
	if err != nil {
		return drkey.SecretValue{}, err
	}

	key, err := getSecretValueFromReply(meta.ProtoId, rep)
	if err != nil {
		return drkey.SecretValue{}, err
	}

	return key, nil
}

func getSecretValueFromReply(proto drkey.Protocol, resp *cppb.DRKeySecretValueResponse) (
	drkey.SecretValue, error) {
	if err := resp.EpochBegin.CheckValid(); err != nil {
		return drkey.SecretValue{}, err
	}
	if err := resp.EpochEnd.CheckValid(); err != nil {
		return drkey.SecretValue{}, err
	}
	epoch := drkey.Epoch{
		Validity: cppki.Validity{
			NotBefore: resp.EpochBegin.AsTime(),
			NotAfter:  resp.EpochEnd.AsTime(),
		},
	}
	sv := drkey.SecretValue{
		ProtoId: proto,
		Epoch:   epoch,
	}
	copy(sv.Key[:], resp.Key)
	return sv, nil
}

func DeriveHostHostKey(sv drkey.SecretValue, meta drkey.HostHostMeta) (
	drkey.HostHostKey, error) {
	var deriver specific.Deriver
	lvl1, err := deriver.DeriveLevel1(meta.DstIA, sv.Key)
	if err != nil {
		return drkey.HostHostKey{}, err
	}
	hostAS, err := deriver.DeriveHostAS(meta.SrcHost, lvl1)
	if err != nil {
		return drkey.HostHostKey{}, err
	}
	hosthost, err := deriver.DeriveHostHost(meta.DstHost, hostAS)
	if err != nil {
		return drkey.HostHostKey{}, err
	}
	return drkey.HostHostKey{
		ProtoId: sv.ProtoId,
		Epoch:   sv.Epoch,
		SrcIA:   meta.SrcIA,
		DstIA:   meta.DstIA,
		SrcHost: meta.SrcHost,
		DstHost: meta.DstHost,
		Key:     hosthost,
	}, nil
}

func FetchHostHostKey(ctx context.Context, dc daemon.Connector, meta drkey.HostHostMeta) (
	drkey.HostHostKey, error) {
	return dc.DRKeyGetHostHostKey(ctx, meta)
}

