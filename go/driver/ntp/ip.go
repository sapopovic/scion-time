package ntp

import (
	"context"
	"log"
	"net"
	"time"

	"example.com/scion-time/go/core/timebase"
	"example.com/scion-time/go/net/ntp"
	"example.com/scion-time/go/net/udp"
)

/*
	var filter struct {
		reference      string
		epoch          uint64
		lo, mid, hi    float64
		alo, amid, ahi float64
		alolo, ahihi   float64
		navg           float64
	}
*/

func MeasureClockOffsetIP(ctx context.Context, localAddr, remoteAddr *net.UDPAddr) (
	offset time.Duration, weight float64, err error) {
	conn, err := net.ListenUDP("udp", &net.UDPAddr{IP: localAddr.IP})
	if err != nil {
		return offset, weight, err
	}
	defer conn.Close()
	deadline, ok := ctx.Deadline()
	if ok {
		err = conn.SetDeadline(deadline)
		if err != nil {
			return offset, weight, err
		}
	}
	udp.EnableTimestamping(conn)

	ntpreq := ntp.Packet{}
	buf := make([]byte, ntp.PacketLen)

	cTxTime := timebase.Now()

	ntpreq.SetVersion(ntp.VersionMax)
	ntpreq.SetMode(ntp.ModeClient)
	ntpreq.TransmitTime = ntp.Time64FromTime(cTxTime)
	ntp.EncodePacket(&buf, &ntpreq)

	_, err = conn.WriteToUDPAddrPort(buf, remoteAddr.AddrPort())
	if err != nil {
		return offset, weight, err
	}

	oob := make([]byte, udp.TimestampLen())
	n, oobn, flags, srcAddr, err := conn.ReadMsgUDPAddrPort(buf, oob)
	if err != nil {
		return offset, weight, err
	}
	if flags != 0 {
		return offset, weight, errUnexpectedPacketFlags
	}

	oob = oob[:oobn]
	cRxTime, err := udp.TimestampFromOOBData(oob)
	if err != nil {
		log.Printf("%s Failed to receive packet timestamp: %v", ntpLogPrefix, err)
		cRxTime = timebase.Now()
	}
	buf = buf[:n]

	var ntpresp ntp.Packet
	err = ntp.DecodePacket(&ntpresp, buf)
	if err != nil {
		return offset, weight, err
	}

	err = ntp.ValidateResponse(&ntpresp, cTxTime)
	if err != nil {
		return offset, weight, err
	}

	log.Printf("%s Received packet at %v from %v: %+v", ntpLogPrefix, cRxTime, srcAddr, ntpresp)

	sRxTime := ntp.TimeFromTime64(ntpresp.ReceiveTime)
	sTxTime := ntp.TimeFromTime64(ntpresp.TransmitTime)

	off := ntp.ClockOffset(cTxTime, sRxTime, sTxTime, cRxTime)
	rtd := ntp.RoundTripDelay(cTxTime, sRxTime, sTxTime, cRxTime)

	log.Printf("%s %s, clock offset: %fs (%fms), round trip delay: %fs (%fms)",
		ntpLogPrefix, remoteAddr,
		float64(off.Nanoseconds())/float64(time.Second.Nanoseconds()),
		float64(off.Nanoseconds())/float64(time.Millisecond.Nanoseconds()),
		float64(rtd.Nanoseconds())/float64(time.Second.Nanoseconds()),
		float64(rtd.Nanoseconds())/float64(time.Millisecond.Nanoseconds()))

	/*
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
	*/

	offset = off
	weight = 1000.0
	return offset, weight, nil
}
