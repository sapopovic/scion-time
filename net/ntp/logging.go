package ntp

import (
	"log/slog"
)

type Time32LogValuer struct {
	T Time32
}

func (v Time32LogValuer) LogValue() slog.Value {
	return slog.GroupValue(
		slog.Uint64("Seconds", uint64(v.T.Seconds)),
		slog.Uint64("Fraction", uint64(v.T.Fraction)),
	)
}

type Time64LogValuer struct {
	T Time64
}

func (v Time64LogValuer) LogValue() slog.Value {
	return slog.GroupValue(
		slog.Uint64("Seconds", uint64(v.T.Seconds)),
		slog.Uint64("Fraction", uint64(v.T.Fraction)),
	)
}

type PacketLogValuer struct {
	Pkt *Packet
}

func (v PacketLogValuer) LogValue() slog.Value {
	return slog.GroupValue(
		slog.Uint64("LVM", uint64(v.Pkt.LVM)),
		slog.Uint64("Stratum", uint64(v.Pkt.Stratum)),
		slog.Int64("Poll", int64(v.Pkt.Poll)),
		slog.Int64("Precision", int64(v.Pkt.Precision)),
		slog.Any("RootDelay", Time32LogValuer{T: v.Pkt.RootDelay}),
		slog.Any("RootDispersion", Time32LogValuer{T: v.Pkt.RootDispersion}),
		slog.Uint64("ReferenceID", uint64(v.Pkt.ReferenceID)),
		slog.Any("ReferenceTime", Time64LogValuer{T: v.Pkt.ReferenceTime}),
		slog.Any("OriginTime", Time64LogValuer{T: v.Pkt.OriginTime}),
		slog.Any("ReceiveTime", Time64LogValuer{T: v.Pkt.ReceiveTime}),
		slog.Any("TransmitTime", Time64LogValuer{T: v.Pkt.TransmitTime}),
	)
}
