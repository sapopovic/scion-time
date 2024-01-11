package ntp_test

import (
	"testing"
	"time"

	"example.com/scion-time/net/ntp"
)

func TestTime64Conversion(t *testing.T) {
	t0 := time.Now()
	t64 := ntp.Time64FromTime(t0)
	t1 := ntp.TimeFromTime64(t64)

	if !t1.Equal(t0) {
		t.Errorf("t1 and t0 must be equal")
	}
}

func TestBeforeAfter(t *testing.T) {
	t0 := ntp.Time64{Seconds: 10, Fraction: 0}
	t1 := ntp.Time64{Seconds: 20, Fraction: 0}

	if !t0.Before(t1) {
		t.Errorf("t0 must be before t1")
	}
	if t1.Before(t0) {
		t.Errorf("t1 must not be before t0")
	}
	if !t1.After(t0) {
		t.Errorf("t1 must be after t0")
	}
	if t0.After(t1) {
		t.Errorf("t0 must not be after t1")
	}
}

func TestBeforeAfterWithFraction(t *testing.T) {
    t0 := ntp.Time64{Seconds: 10, Fraction: 0}
    t1 := ntp.Time64{Seconds: 20, Fraction: 0}
    t2 := ntp.Time64{Seconds: 10, Fraction: 100}
    t3 := ntp.Time64{Seconds: 10, Fraction: 200}

    // Testing with Fraction = 0
    if !t0.Before(t1) {
        t.Errorf("t0 must be before t1")
    }
    if t1.Before(t0) {
        t.Errorf("t1 must not be before t0")
    }
    if !t1.After(t0) {
        t.Errorf("t1 must be after t0")
    }
    if t0.After(t1) {
        t.Errorf("t0 must not be after t1")
    }

    // Testing with non-zero Fraction
    if !t2.Before(t3) {
        t.Errorf("t2 must be before t3")
    }
    if t3.Before(t2) {
        t.Errorf("t3 must not be before t2")
    }
    if !t3.After(t2) {
        t.Errorf("t3 must be after t2")
    }
    if t2.After(t3) {
        t.Errorf("t2 must not be after t3")
    }
}

