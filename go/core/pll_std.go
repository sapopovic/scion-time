package core

import (
	"log"
	"time"
)

const stdPLLLogPrefix = "[core/pll_std]"

type StandardPLL struct{}

var _ PLL = (*StandardPLL)(nil)

func (l *StandardPLL) Do(offset time.Duration, weight float64) {
	log.Printf("%s core.StandardPLL.Do", stdPLLLogPrefix)
}
