package filter

import "time"

const (
	DefaultFilterSize = 16
	DefaultFilterPick = 1
)

//lint:ignore U1000 WIP
type measurement struct {
	cTxTime time.Time
	sRxTime time.Time
	sTxTime time.Time
	cRxTime time.Time
}

type Filter interface {
	Size() int
	Pick() int
	Do(reference string, cTxTime, sRxTime, sTxTime, cRxTime time.Time) (offset time.Duration)
}
