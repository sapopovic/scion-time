package drivers

import (
	"unsafe"

	"encoding/binary"
	"fmt"
	"log"
	"net"
	"time"

	"golang.org/x/sys/unix"

	"github.com/facebook/time/ntp/protocol/ntp"
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

	err = ntp.EnableKernelTimestampsSocket(conn.(*net.UDPConn))
	if err != nil {
		return
	}

	clientTxTime := time.Now().UTC()
	sec, frac := ntp.Time(clientTxTime)
	request := &ntp.Packet{
		Settings:   0x1B,
		TxTimeSec:  sec,
		TxTimeFrac: frac,
	}
	err = binary.Write(conn, binary.BigEndian, request)
	if err != nil {
		return
	}

	buf := make([]byte, ntp.PacketSizeBytes)
	oob := make([]byte, ntp.ControlHeaderSizeBytes)
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
	response, err := ntp.BytesToPacket(buf)
	if err != nil {
		log.Printf("%s Failed to decode packet payload: %v", ntpLogPrefix, err)
		return
	}	

	serverRxTime := ntp.Unix(response.RxTimeSec, response.RxTimeFrac)
	serverTxTime := ntp.Unix(response.TxTimeSec, response.TxTimeFrac)

	avgNetworkDelay := ntp.AvgNetworkDelay(clientTxTime, serverRxTime, serverTxTime, clientRxTime)
	refTime = ntp.CurrentRealTime(serverTxTime, avgNetworkDelay)
	sysTime = time.Now().UTC()

	log.Printf("%s Received NTP packet from %s: %+v", ntpLogPrefix, host, response)

	offset := ntp.CalculateOffset(refTime, sysTime)
	log.Printf("%s Offset: %fs (%fms), Network delay: %fs (%fms)",
		ntpLogPrefix,
		float64(offset)/float64(time.Second.Nanoseconds()),
		float64(offset)/float64(time.Millisecond.Nanoseconds()),
		float64(avgNetworkDelay)/float64(time.Second.Nanoseconds()),
		float64(avgNetworkDelay)/float64(time.Millisecond.Nanoseconds()))

	return
}
