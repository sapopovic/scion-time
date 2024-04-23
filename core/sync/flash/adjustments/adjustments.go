package adjustments

import "time"

type Adjustment interface {
	Do(offset time.Duration, drift float64)
}
