package core

import (
	"time"
)

type PLL interface {
	Do(offset time.Duration, weight float64)
}
