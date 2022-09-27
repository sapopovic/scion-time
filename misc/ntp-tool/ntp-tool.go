package main

import (
	"log"
	"os"
	"time"

	"github.com/beevik/ntp"
)

func main() {
	host := os.Args[1]

	log.Printf("[%s] ----------------------", host)
	log.Printf("[%s] NTP protocol version %d", host, 4)

	r, err := ntp.QueryWithOptions(host, ntp.QueryOptions{Timeout: 3 * time.Second})
	if err != nil {
		log.Fatal("Failed to query NTP server:", err)
	}
	err = r.Validate()
	if err != nil {
		log.Fatal("Failed to validate NTP response:", err)
	}

	now := time.Now()

	log.Printf("[%s]  LocalTime: %v", host, now)
	log.Printf("[%s]   XmitTime: %v", host, r.Time)
	log.Printf("[%s]    RefTime: %v", host, r.ReferenceTime)
	log.Printf("[%s]        RTT: %v", host, r.RTT)
	log.Printf("[%s]     Offset: %v", host, r.ClockOffset)
	log.Printf("[%s]       Poll: %v", host, r.Poll)
	log.Printf("[%s]  Precision: %v", host, r.Precision)
	log.Printf("[%s]    Stratum: %v", host, r.Stratum)
	log.Printf("[%s]      RefID: 0x%08x", host, r.ReferenceID)
	log.Printf("[%s]  RootDelay: %v", host, r.RootDelay)
	log.Printf("[%s]   RootDisp: %v", host, r.RootDispersion)
	log.Printf("[%s]   RootDist: %v", host, r.RootDistance)
	log.Printf("[%s]   MinError: %v", host, r.MinError)
	log.Printf("[%s]       Leap: %v", host, r.Leap)
	log.Printf("[%s]   KissCode: \"%v\"", host, r.KissCode)
}