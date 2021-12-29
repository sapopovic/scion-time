package drivers

import (
	"unsafe"

	"fmt"
	"log"
	"math"
	"net"
	"time"

	"golang.org/x/sys/unix"

	fbntp "github.com/facebook/time/ntp/protocol/ntp"

	"example.com/scion-time/go/protocol/ntp"
)

const ntpLogPrefix = "[drivers/ntp]"

func FetchNTPTime(host string) (refTime time.Time, sysTime time.Time, err error) {
	refTime = time.Time{}
	sysTime = time.Time{}

	const timeout = 5 * time.Second
	addr := net.JoinHostPort(host, "123")
	conn, err := net.DialTimeout("udp", addr, timeout)
	if err != nil {
		return
	}
	defer conn.Close()

	err = fbntp.EnableKernelTimestampsSocket(conn.(*net.UDPConn))
	if err != nil {
		return
	}

	pkt := ntp.Packet{}
	buf := make([]byte, ntp.PacketLen)
	oob := make([]byte, fbntp.ControlHeaderSizeBytes)
	var n, oobn int

	clientTxTime := time.Now().UTC()

	ntp.SetLeapIndicator(&pkt.LVM, ntp.LeapIndicatorUnknown)
	ntp.SetVersion(&pkt.LVM, ntp.VersionMax)
	ntp.SetMode(&pkt.LVM, ntp.ModeClient)
	pkt.TransmitTime = ntp.Time64FromTime(clientTxTime)
	ntp.EncodePacket(&pkt, &buf)

	_, err = conn.Write(buf)
	if err != nil {
		return
	}

	blockingRead := make(chan bool, 1)
	go func() {
		n, oobn, _, _, err = conn.(*net.UDPConn).ReadMsgUDP(buf, oob)
		blockingRead <- true
	}()
	select {
	case <-blockingRead:
		if err != nil {
			return
		}
	case <-time.After(timeout):
		err = fmt.Errorf("timeout waiting for reply from server for %v", timeout)
		return
	}

	var clientRxTime time.Time
	if oobn != 0 {
		ts := (*unix.Timespec)(unsafe.Pointer(&oob[unix.CmsgSpace(0)]))
		clientRxTime = time.Unix(ts.Unix())
	} else {
		log.Printf("Failed to receive kernel timestamp")
		clientRxTime = time.Now().UTC()
	}
	buf = buf[:n]
	response, err := fbntp.BytesToPacket(buf)
	if err != nil {
		log.Printf("%s Failed to decode packet payload: %v", ntpLogPrefix, err)
		return
	}

	err = ntp.DecodePacket(buf, &pkt)
	if err != nil {
		log.Printf("%s Failed to decode packet payload: %v", ntpLogPrefix, err)
		return
	}

	if pkt.LVM != response.Settings ||
		pkt.Stratum != response.Stratum ||
		pkt.Poll != response.Poll ||
		pkt.Precision != response.Precision ||
		pkt.RootDelay.Seconds != uint16(response.RootDelay >> 16) ||
		pkt.RootDelay.Fraction != uint16(response.RootDelay) ||
		pkt.RootDispersion.Seconds != uint16(response.RootDispersion >> 16) ||
		pkt.RootDispersion.Fraction != uint16(response.RootDispersion) ||
		pkt.ReferenceID != response.ReferenceID ||
		pkt.ReferenceTime.Seconds != response.RefTimeSec ||
		pkt.ReferenceTime.Fraction != response.RefTimeFrac ||
		pkt.OriginTime.Seconds != response.OrigTimeSec ||
		pkt.OriginTime.Fraction != response.OrigTimeFrac ||
		pkt.ReceiveTime.Seconds != response.RxTimeSec ||
		pkt.ReceiveTime.Fraction != response.RxTimeFrac ||
		pkt.TransmitTime.Seconds != response.TxTimeSec ||
		pkt.TransmitTime.Fraction != response.TxTimeFrac {
		panic("NTP packet decoder error")
	}
	log.Printf("%s NTP packet decoder check passed", ntpLogPrefix)

	serverRxTime := ntp.TimeFromTime64(pkt.ReceiveTime)
	serverTxTime := ntp.TimeFromTime64(pkt.TransmitTime)

	if math.Abs(serverRxTime.Sub(fbntp.Unix(response.RxTimeSec, response.RxTimeFrac)).Seconds()) > 1e-9 {
		panic("NTP timestamp converter error")
	}
	if math.Abs(serverTxTime.Sub(fbntp.Unix(response.TxTimeSec, response.TxTimeFrac)).Seconds()) > 1e-9 {
		panic("NTP timestamp converter error")
	}

	avgNetworkDelay := fbntp.AvgNetworkDelay(clientTxTime, serverRxTime, serverTxTime, clientRxTime)
	refTime = fbntp.CurrentRealTime(serverTxTime, avgNetworkDelay)
	sysTime = time.Now().UTC()

	log.Printf("%s Received NTP packet from %s: %+v", ntpLogPrefix, host, response)

	offset := fbntp.CalculateOffset(refTime, sysTime)
	log.Printf("%s Offset: %fs (%fms), Network delay: %fs (%fms)",
		ntpLogPrefix,
		float64(offset)/float64(time.Second.Nanoseconds()),
		float64(offset)/float64(time.Millisecond.Nanoseconds()),
		float64(avgNetworkDelay)/float64(time.Second.Nanoseconds()),
		float64(avgNetworkDelay)/float64(time.Millisecond.Nanoseconds()))

	clockOffset := ntp.ClockOffset(clientTxTime, serverRxTime, serverTxTime, clientRxTime).Nanoseconds()
	roundTripDelay := ntp.RoundTripDelay(clientTxTime, serverRxTime, serverTxTime, clientRxTime).Nanoseconds()

	if math.Abs(float64(clockOffset - offset)) > 1e6 {
		panic("NTP timestamp calculator error")
	}

	log.Printf("%s Clock offset: %fs (%fms), round trip delay: %fs (%fms)",
		ntpLogPrefix,
		float64(clockOffset)/float64(time.Second.Nanoseconds()),
		float64(clockOffset)/float64(time.Millisecond.Nanoseconds()),
		float64(roundTripDelay)/float64(time.Second.Nanoseconds()),
		float64(roundTripDelay)/float64(time.Millisecond.Nanoseconds()))

	return
}
