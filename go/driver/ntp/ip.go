package ntp

import (
	"context"
	"log"
	"net"
	"net/netip"
	"time"

	"example.com/scion-time/go/core/timebase"
	"example.com/scion-time/go/net/ntp"
	"example.com/scion-time/go/net/udp"
)

type IPClient struct{
	Interleaved bool
	prev struct {
		cTxTime time.Time
		cRxTime time.Time
		sRxTime time.Time
	}
}

var DefaultIPClient = &IPClient{}

func compareAddrs(x, y netip.Addr) int {
	if x.Is4In6() {
		x = netip.AddrFrom4(x.As4())
	}
	if y.Is4In6() {
		y = netip.AddrFrom4(y.As4())
	}
	return x.Compare(y)
}

func (c *IPClient) MeasureClockOffsetIP(ctx context.Context, localAddr, remoteAddr *net.UDPAddr) (
	offset time.Duration, weight float64, err error) {
	conn, err := net.ListenUDP("udp", &net.UDPAddr{IP: localAddr.IP})
	if err != nil {
		return offset, weight, err
	}
	defer conn.Close()
	deadline, deadlineIsSet := ctx.Deadline()
	if deadlineIsSet {
		err = conn.SetDeadline(deadline)
		if err != nil {
			return offset, weight, err
		}
	}
	udp.EnableTimestamping(conn)

	buf := make([]byte, ntp.PacketLen)

	cTxTime0 := timebase.Now()

	ntpreq := ntp.Packet{}
	ntpreq.SetVersion(ntp.VersionMax)
	ntpreq.SetMode(ntp.ModeClient)
	if c.Interleaved {
		ntpreq.OriginTime = ntp.Time64FromTime(c.prev.sRxTime)
		ntpreq.ReceiveTime = ntp.Time64FromTime(c.prev.cRxTime)
		ntpreq.TransmitTime = ntp.Time64FromTime(c.prev.cTxTime)
	} else {
		ntpreq.TransmitTime = ntp.Time64FromTime(cTxTime0)
	}
	ntp.EncodePacket(&buf, &ntpreq)

	_, err = conn.WriteToUDPAddrPort(buf, remoteAddr.AddrPort())
	if err != nil {
		return offset, weight, err
	}
	cTxTime1 := timebase.Now()

	oob := make([]byte, udp.TimestampLen())
	for {
		buf = buf[:cap(buf)]
		oob = oob[:cap(oob)]
		n, oobn, flags, srcAddr, err := conn.ReadMsgUDPAddrPort(buf, oob)
		if err != nil {
			if deadlineIsSet && timebase.Now().Before(deadline) {
				log.Printf("%s Failed to receive packet: %v", ntpLogPrefix, err)
				continue
			}
			return offset, weight, err
		}
		if flags != 0 {
			err = errUnexpectedPacketFlags
			if deadlineIsSet && timebase.Now().Before(deadline) {
				log.Printf("%s Failed to receive packet: %v", ntpLogPrefix, err)
				continue
			}
			return offset, weight, err
		}
		oob = oob[:oobn]
		cRxTime, err := udp.TimestampFromOOBData(oob)
		if err != nil {
			cRxTime = timebase.Now()
			log.Printf("%s Failed to receive packet timestamp: %v", ntpLogPrefix, err)
		}
		buf = buf[:n]

		if compareAddrs(srcAddr.Addr(), remoteAddr.AddrPort().Addr()) != 0 {
			err = errUnexpectedPacket
			if deadlineIsSet && timebase.Now().Before(deadline) {
				log.Printf("%s Failed to receive packet: %v", ntpLogPrefix, err)
				continue
			}
			return offset, weight, err
		}

		var ntpresp ntp.Packet
		err = ntp.DecodePacket(&ntpresp, buf)
		if err != nil {
			if deadlineIsSet && timebase.Now().Before(deadline) {
				log.Printf("%s Failed to receive packet: %v", ntpLogPrefix, err)
				continue
			}
			return offset, weight, err
		}

		interleaved := false
		if c.Interleaved && ntpresp.OriginTime == ntp.Time64FromTime(c.prev.cRxTime) {
			interleaved = true
		} else if ntpresp.OriginTime != ntp.Time64FromTime(cTxTime0) {
			err = errUnexpectedPacket
			if deadlineIsSet && timebase.Now().Before(deadline) {
				log.Printf("%s Failed to receive packet: %v", ntpLogPrefix, err)
				continue
			}
			return offset, weight, err
		}

		err = ntp.ValidateMetadata(&ntpresp)
		if err != nil {
			return offset, weight, err
		}

		log.Printf("%s Received packet at %v from %v: %+v", ntpLogPrefix, cRxTime, srcAddr, ntpresp)

		sRxTime := ntp.TimeFromTime64(ntpresp.ReceiveTime)
		sTxTime := ntp.TimeFromTime64(ntpresp.TransmitTime)

		var t0, t1, t2, t3 time.Time
		if interleaved {
			t0 = c.prev.cTxTime
			t1 = c.prev.sRxTime
			t2 = sTxTime
			t3 = c.prev.cRxTime
		} else {
			t0 = cTxTime1
			t1 = sRxTime
			t2 = sTxTime
			t3 = cRxTime
		}

		err = ntp.ValidateTimestamps(t0, t1, t1, t3)
		if err != nil {
			return offset, weight, err
		}

		off := ntp.ClockOffset(t0, t1, t2, t3)
		rtd := ntp.RoundTripDelay(t0, t1, t2, t3)

		log.Printf("%s %s, clock offset: %fs (%fms), round trip delay: %fs (%fms)",
			ntpLogPrefix, remoteAddr,
			float64(off.Nanoseconds())/float64(time.Second.Nanoseconds()),
			float64(off.Nanoseconds())/float64(time.Millisecond.Nanoseconds()),
			float64(rtd.Nanoseconds())/float64(time.Second.Nanoseconds()),
			float64(rtd.Nanoseconds())/float64(time.Millisecond.Nanoseconds()))

		c.prev.cTxTime = cTxTime1
		c.prev.cRxTime = cRxTime
		c.prev.sRxTime = sRxTime

		// offset, weight = off, 1000.0

		reference := remoteAddr.String()
		offset, weight = filter(reference, t0, t1, t2, t3)

		break
	}

	return offset, weight, nil
}

func MeasureClockOffsetIP(ctx context.Context, localAddr, remoteAddr *net.UDPAddr) (
	offset time.Duration, weight float64, err error) {
	return DefaultIPClient.MeasureClockOffsetIP(ctx, localAddr, remoteAddr)
}
