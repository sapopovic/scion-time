package adjustments

import "time"

const (
	stepLimit = 500000000 * time.Nanosecond
)

type Adjustment interface {
	Do(offset time.Duration) error
}
