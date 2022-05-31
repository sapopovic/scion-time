package benchmark

import (
	"log"
	"net"
	"os"
	"sync"
	"time"

	"github.com/HdrHistogram/hdrhistogram-go"

	"example.com/scion-time/go/core"
	"example.com/scion-time/go/core/timebase"

	"example.com/scion-time/go/net/ntp"
	"example.com/scion-time/go/net/udp"
)

func RunIPBenchmark(localAddr, remoteAddr *net.UDPAddr) {
	lclk := &core.SystemClock{}
	timebase.RegisterClock(lclk)

	// const numClientGoroutine = 8
	// const numRequestPerClient = 10000
	const numClientGoroutine = 1
	const numRequestPerClient = 10000 * 100
	var mu sync.Mutex
	sg := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(numClientGoroutine)
	for i := numClientGoroutine; i > 0; i-- {
		go func() {
			hg := hdrhistogram.New(1, 50000, 5)

			conn, err := net.DialUDP("udp", localAddr, remoteAddr)
			if err != nil {
				log.Printf("Failed to dial UDP connection: %v", err)
				return
			}
			defer conn.Close()
			udp.EnableTimestamping(conn)

			defer wg.Done()
			<-sg
			for j := numRequestPerClient; j > 0; j-- {
				ntpreq := ntp.Packet{}
				buf := make([]byte, ntp.PacketLen)

				cTxTime := timebase.Now()

				ntpreq.SetVersion(ntp.VersionMax)
				ntpreq.SetMode(ntp.ModeClient)
				ntpreq.TransmitTime = ntp.Time64FromTime(cTxTime)
				ntp.EncodePacket(&buf, &ntpreq)

				_, err = conn.Write(buf)
				if err != nil {
					log.Printf("Failed to write packet: %v", err)
					return
				}

				oob := make([]byte, udp.TimestampLen())

				n, oobn, flags, _, err := conn.ReadMsgUDPAddrPort(buf, oob)
				if err != nil {
					log.Printf("Failed to read packet: %v", err)
					return
				}
				if flags != 0 {
					log.Printf("Failed to read packet, flags: %v", flags)
					return
				}

				oob = oob[:oobn]
				cRxTime, err := udp.TimestampFromOOBData(oob)
				if err != nil {
					log.Printf("Failed to read packet timestamp")
					cRxTime = timebase.Now()
				}
				buf = buf[:n]

				var ntpresp ntp.Packet
				err = ntp.DecodePacket(&ntpresp, buf)
				if err != nil {
					log.Printf("Failed to decode packet payload: %v", err)
					return
				}

				sRxTime := ntp.TimeFromTime64(ntpresp.ReceiveTime)
				sTxTime := ntp.TimeFromTime64(ntpresp.TransmitTime)

				_ = ntp.ClockOffset(cTxTime, sRxTime, sTxTime, cRxTime)
				roundTripDelay := ntp.RoundTripDelay(cTxTime, sRxTime, sTxTime, cRxTime)

				hg.RecordValue(roundTripDelay.Microseconds())
			}
			mu.Lock()
    	defer mu.Unlock()
    	hg.PercentilesPrint(os.Stdout, 1, 1.0)
		}()
	}
	t0 := time.Now()
	close(sg)
	wg.Wait()
	log.Print(time.Since(t0))
}
