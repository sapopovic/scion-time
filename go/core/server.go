package core

import (
	"fmt"
	"time"

	"example.com/scion-time/go/core/timebase"
	"example.com/scion-time/go/net/ntp"
)

var errUnexpectedPacket = fmt.Errorf("failed to validate request")

func validateRequest(req *ntp.Packet, srcPort uint16) error {
	li := req.LeapIndicator()
	if li != ntp.LeapIndicatorNoWarning && li != ntp.LeapIndicatorUnknown {
		return errUnexpectedPacket
	}
	vn := req.Version()
	if vn < ntp.VersionMin || ntp.VersionMax < vn {
		return errUnexpectedPacket
	}
	mode := req.Mode()
	if vn == 1 && mode != ntp.ModeReserved0 || vn != 1 && mode != ntp.ModeClient {
		return errUnexpectedPacket
	}
	if vn == 1 && srcPort == ntp.ServerPort {
		return errUnexpectedPacket
	}
	return nil
}

func handleRequest(req *ntp.Packet, rxt time.Time, resp *ntp.Packet) {
	txt := timebase.Now()

	resp.SetVersion(ntp.VersionMax)
	resp.SetMode(ntp.ModeServer)
	resp.Stratum = 1
	resp.Poll = req.Poll
	resp.Precision = -32
	resp.RootDispersion = ntp.Time32{Seconds: 0, Fraction: 10}
	resp.ReferenceID = ntp.ServerRefID

	resp.ReferenceTime = ntp.Time64FromTime(txt)
	resp.OriginTime = req.TransmitTime
	resp.ReceiveTime = ntp.Time64FromTime(rxt)
	resp.TransmitTime = ntp.Time64FromTime(txt)
}
