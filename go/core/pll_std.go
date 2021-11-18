package core

import (
	"log"
	"time"
)

type stdPLL struct {}

func (l *stdPLL) Do(offset time.Duration, weight float64) {
	log.Printf("core.stdPLL.Do")
}

func NewStdPLL() PLL {
	return &stdPLL{}
}
