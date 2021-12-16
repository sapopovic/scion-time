package drivers

import (
	"log"
	"time"

	"github.com/beevik/ntp"
)

const ntpLogPrefix = "[drivers/ntp]"

func FetchNTPTime(host string) (refTime time.Time, sysTime time.Time, err error) {
	log.Printf("%s [%s] ----------------------", ntpLogPrefix, host)
	log.Printf("%s [%s] NTP protocol version %d", ntpLogPrefix, host, 4)

	r, err := ntp.QueryWithOptions(host, ntp.QueryOptions{Timeout: 3 * time.Second})
	if err != nil {
		return time.Time{}, time.Time{}, err
	}
	err = r.Validate()
	if err != nil {
		return time.Time{}, time.Time{}, err
	}

	now := time.Now()

	log.Printf("%s [%s]  LocalTime: %v", ntpLogPrefix, host, now)
	log.Printf("%s [%s]   XmitTime: %v", ntpLogPrefix, host, r.Time)
	log.Printf("%s [%s]    RefTime: %v", ntpLogPrefix, host, r.ReferenceTime)
	log.Printf("%s [%s]        RTT: %v", ntpLogPrefix, host, r.RTT)
	log.Printf("%s [%s]     Offset: %v", ntpLogPrefix, host, r.ClockOffset)
	log.Printf("%s [%s]       Poll: %v", ntpLogPrefix, host, r.Poll)
	log.Printf("%s [%s]  Precision: %v", ntpLogPrefix, host, r.Precision)
	log.Printf("%s [%s]    Stratum: %v", ntpLogPrefix, host, r.Stratum)
	log.Printf("%s [%s]      RefID: 0x%08x", ntpLogPrefix, host, r.ReferenceID)
	log.Printf("%s [%s]  RootDelay: %v", ntpLogPrefix, host, r.RootDelay)
	log.Printf("%s [%s]   RootDisp: %v", ntpLogPrefix, host, r.RootDispersion)
	log.Printf("%s [%s]   RootDist: %v", ntpLogPrefix, host, r.RootDistance)
	log.Printf("%s [%s]   MinError: %v", ntpLogPrefix, host, r.MinError)
	log.Printf("%s [%s]       Leap: %v", ntpLogPrefix, host, r.Leap)
	log.Printf("%s [%s]   KissCode: \"%v\"", ntpLogPrefix, host, r.KissCode)

	return r.ReferenceTime, now, nil
}
