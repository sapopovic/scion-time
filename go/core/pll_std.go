package core

import (
	"log"
	"time"
)

const stdPLLLogPrefix = "[core/pll_std]"

type StdPLL struct {}

var _ PLL = (*StdPLL)(nil)

func (l *StdPLL) Do(offset time.Duration, weight float64) {
	log.Printf("%s core.StdPLL.Do", stdPLLLogPrefix)
}
