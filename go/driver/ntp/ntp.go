package ntp

import (
	"context"
	"fmt"
	"log"
	"math"
	"net"
	"time"

	"example.com/scion-time/go/core"
	"example.com/scion-time/go/core/timebase"
	"example.com/scion-time/go/core/timemath"
	"example.com/scion-time/go/net/ntp"
	"example.com/scion-time/go/net/udp"
)

const (
	ntpLogPrefix = "[driver/ntp]"

	timeout = 5 * time.Second
)

var (
	errUnexpectedPacketFlags = fmt.Errorf("failed to read packet: unexpected flags")

	filter struct {
		reference      string
		epoch          uint64
		lo, mid, hi    float64
		alo, amid, ahi float64
		alolo, ahihi   float64
		navg           float64
	}
)

func MeasureClockOffset(ctx context.Context, host string) (time.Duration, float64, error) {
	now := timebase.Now()
	deadline, ok := ctx.Deadline()
	if !ok {
		deadline = now.Add(timeout)
	}
	addr := net.JoinHostPort(host, "123")
	conn, err := net.DialTimeout("udp", addr, deadline.Sub(now))
	if err != nil {
		return 0, 0.0, err
	}
	defer conn.Close()
	err = conn.SetDeadline(deadline)
	if err != nil {
		return 0, 0.0, err
	}
	udpConn := conn.(*net.UDPConn)
	udp.EnableTimestamping(udpConn)

	pkt := ntp.Packet{}
	buf := make([]byte, ntp.PacketLen)
	oob := make([]byte, udp.TimestampLen())

	cTxTime := timebase.Now()

	pkt.SetVersion(ntp.VersionMax)
	pkt.SetMode(ntp.ModeClient)
	pkt.TransmitTime = ntp.Time64FromTime(cTxTime)
	ntp.EncodePacket(&buf, &pkt)

	_, err = udpConn.Write(buf)
	if err != nil {
		return 0, 0.0, err
	}
	n, oobn, flags, srcAddr, err := udpConn.ReadMsgUDPAddrPort(buf, oob)
	if err != nil {
		log.Printf("%s Failed to read packet: %v", ntpLogPrefix, err)
		return 0, 0.0, err
	}
	if flags != 0 {
		log.Printf("%s Failed to read packet, flags: %v", ntpLogPrefix, flags)
		return 0, 0.0, errUnexpectedPacketFlags
	}

	oob = oob[:oobn]
	cRxTime, err := udp.TimestampFromOOBData(oob)
	if err != nil {
		log.Printf("%s %s, failed to read packet timestamp", ntpLogPrefix, host, err)
		cRxTime = timebase.Now()
	}
	buf = buf[:n]

	err = ntp.DecodePacket(&pkt, buf)
	if err != nil {
		log.Printf("%s %s, failed to decode packet payload: %v", ntpLogPrefix, host, err)
		return 0, 0.0, err
	}

	log.Printf("%s %s, received packet at %v from srcAddr %v: %+v", ntpLogPrefix, host, cRxTime, srcAddr, pkt)

	sRxTime := ntp.TimeFromTime64(pkt.ReceiveTime)
	sTxTime := ntp.TimeFromTime64(pkt.TransmitTime)

	off := ntp.ClockOffset(cTxTime, sRxTime, sTxTime, cRxTime)
	rtd := ntp.RoundTripDelay(cTxTime, sRxTime, sTxTime, cRxTime)

	log.Printf("%s %s, clock offset: %fs (%fms), round trip delay: %fs (%fms)",
		ntpLogPrefix, host,
		float64(off.Nanoseconds())/float64(time.Second.Nanoseconds()),
		float64(off.Nanoseconds())/float64(time.Millisecond.Nanoseconds()),
		float64(rtd.Nanoseconds())/float64(time.Second.Nanoseconds()),
		float64(rtd.Nanoseconds())/float64(time.Millisecond.Nanoseconds()))

	if filter.reference == "" {
		filter.reference = host
	} else if filter.reference != host {
		panic("unexpected reference host")
	}

	lo := timemath.Seconds(cTxTime.Sub(sRxTime))
	hi := timemath.Seconds(cRxTime.Sub(sTxTime))
	mid := (lo + hi) / 2

	if filter.epoch != timebase.Epoch() {
		filter.epoch = timebase.Epoch()
		filter.alo = 0.0
		filter.amid = 0.0
		filter.ahi = 0.0
		filter.alolo = 0.0
		filter.ahihi = 0.0
		filter.navg = 0.0
	}

	const (
		filterAverage   = 20.0
		filterThreshold = 3.0
	)

	if filter.navg < filterAverage {
		filter.navg += 1.0
	}

	var loNoise, hiNoise float64
	if filter.navg > 2.0 {
		loNoise = math.Sqrt(filter.alolo - filter.alo*filter.alo)
		hiNoise = math.Sqrt(filter.ahihi - filter.ahi*filter.ahi)
	}

	loLim := filter.alo - loNoise*filterThreshold
	hiLim := filter.ahi + hiNoise*filterThreshold

	var branch int
	failLo := lo < loLim
	failHi := hi > hiLim
	if failLo && failHi {
		branch = 1
	} else if filter.navg > 3.0 && failLo {
		mid = filter.amid + (hi - filter.ahi)
		branch = 2
	} else if filter.navg > 3.0 && failHi {
		mid = filter.amid + (lo - filter.alo)
		branch = 3
	} else {
		branch = 4
	}

	r := filter.navg
	if filter.navg > 2.0 && branch != 4 {
		r *= r
	}

	filter.alo += (lo - filter.alo) / r
	filter.amid += (mid - filter.amid) / r
	filter.ahi += (hi - filter.ahi) / r
	filter.alolo += (lo*lo - filter.alolo) / r
	filter.ahihi += (hi*hi - filter.ahihi) / r

	trust := 1.0

	offset, weight := core.Combine(timemath.Duration(lo), timemath.Duration(mid), timemath.Duration(hi), trust)

	log.Printf("%s %s, %v, %fs, %fs, %fs, %fs, %fs, %fs; offeset=%fs, weight=%f",
		ntpLogPrefix, host, branch,
		lo, mid, hi,
		loLim, filter.amid, hiLim,
		timemath.Seconds(offset), weight)

	return offset, weight, nil
}
