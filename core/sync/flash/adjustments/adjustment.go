package adjustments

import "time"

type Adjustment interface {
	Do(offset, drift time.Duration)
}
