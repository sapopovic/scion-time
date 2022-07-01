package ntp

import (
	"fmt"
	"time"

	"example.com/scion-time/go/core/timebase"
)

var errUnexpectedPacket = fmt.Errorf("failed to validate request")

func ValidateRequest(req *Packet, srcPort uint16) error {
	li := req.LeapIndicator()
	if li != LeapIndicatorNoWarning && li != LeapIndicatorUnknown {
		return errUnexpectedPacket
	}
	vn := req.Version()
	if vn < VersionMin || VersionMax < vn {
		return errUnexpectedPacket
	}
	mode := req.Mode()
	if vn == 1 && mode != ModeReserved0 || vn != 1 && mode != ModeClient {
		return errUnexpectedPacket
	}
	if vn == 1 && srcPort == ServerPort {
		return errUnexpectedPacket
	}
	return nil
}

func HandleRequest(req *Packet, rxt time.Time, resp *Packet) {
	txt := timebase.Now()

	resp.SetVersion(VersionMax)
	resp.SetMode(ModeServer)
	resp.Stratum = 1
	resp.Poll = req.Poll
	resp.Precision = -32
	resp.RootDispersion = Time32{Seconds: 0, Fraction: 10}
	resp.ReferenceID = ServerRefID

	resp.ReferenceTime = Time64FromTime(txt)
	resp.OriginTime = req.TransmitTime
	resp.ReceiveTime = Time64FromTime(rxt)
	resp.TransmitTime = Time64FromTime(txt)
}
