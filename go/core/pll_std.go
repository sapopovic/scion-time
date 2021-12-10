package core

import (
	"log"
	"time"
)

type StdPLL struct {}

var _ PLL = (*StdPLL)(nil)

func (l *StdPLL) Do(offset time.Duration, weight float64) {
	log.Printf("core.StdPLL.Do")
}
