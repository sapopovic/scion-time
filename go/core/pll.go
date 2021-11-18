package core

import (
	"time"
)

type PLL interface {
	Do(offset time.Duration, weight float64)
}

var pll PLL

func PLLInstance() PLL {
	if pll == nil {
		panic("No PLL registered")
	}
	return pll
}

func RegisterPLL(l PLL) {
	if l == nil {
		panic("PLL must not be nil")
	}
	if pll != nil {
		panic("PLL already registered")
	}
	pll = l
}


