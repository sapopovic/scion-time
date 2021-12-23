package drivers

import (
	"unsafe"

	"encoding/binary"
	"fmt"
	"log"
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

	clientTxTime := time.Now().UTC()
	sec, frac := fbntp.Time(clientTxTime)
	request := &fbntp.Packet{
		Settings:   0x1B,
		TxTimeSec:  sec,
		TxTimeFrac: frac,
	}
	err = binary.Write(conn, binary.BigEndian, request)
	if err != nil {
		return
	}

	buf := make([]byte, fbntp.PacketSizeBytes)
	oob := make([]byte, fbntp.ControlHeaderSizeBytes)
	var n, oobn int

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

	var ntpreq ntp.Packet
	err = ntp.DecodePacket(buf, &ntpreq)
	if err != nil {
		log.Printf("%s Failed to decode packet payload: %v", ntpLogPrefix, err)
		return
	}

	if ntpreq.LIVNMode != response.Settings ||
		ntpreq.Stratum != response.Stratum ||
		ntpreq.Poll != response.Poll ||
		ntpreq.Precision != response.Precision ||
		ntpreq.RootDelay.Seconds != uint16(response.RootDelay >> 16) ||
		ntpreq.RootDelay.Fraction != uint16(response.RootDelay) ||
		ntpreq.RootDispersion.Seconds != uint16(response.RootDispersion >> 16) ||
		ntpreq.RootDispersion.Fraction != uint16(response.RootDispersion) ||
		ntpreq.ReferenceID != response.ReferenceID ||
		ntpreq.ReferenceTime.Seconds != response.RefTimeSec ||
		ntpreq.ReferenceTime.Fraction != response.RefTimeFrac ||
		ntpreq.OriginTime.Seconds != response.OrigTimeSec ||
		ntpreq.OriginTime.Fraction != response.OrigTimeFrac ||
		ntpreq.ReceiveTime.Seconds != response.RxTimeSec ||
		ntpreq.ReceiveTime.Fraction != response.RxTimeFrac ||
		ntpreq.TransmitTime.Seconds != response.TxTimeSec ||
		ntpreq.TransmitTime.Fraction != response.TxTimeFrac {
		panic("NTP packet decoder error")
	}
	log.Printf("%s NTP packet decoder check passed", ntpLogPrefix)	

	serverRxTime := fbntp.Unix(response.RxTimeSec, response.RxTimeFrac)
	serverTxTime := fbntp.Unix(response.TxTimeSec, response.TxTimeFrac)

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

	return
}
