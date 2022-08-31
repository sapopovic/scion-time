//go:build linux

package core

import (
	"unsafe"

	"fmt"
	"log"
	"math"
	"time"

	"golang.org/x/sys/unix"

	"example.com/scion-time/go/core/timebase"
)

const sysClockLogPrefix = "[core/clock_sys_linux]"

type SystemClock struct{}

var _ timebase.LocalClock = (*SystemClock)(nil)

func (c *SystemClock) Now() time.Time {
	var ts unix.Timespec
	err := unix.ClockGettime(unix.CLOCK_REALTIME, &ts)
	if err != nil {
		panic("unix.ClockGettime failed")
	}
	return time.Unix(ts.Unix()).UTC()
}

func (c *SystemClock) MaxDrift(duration time.Duration) time.Duration {
	return math.MaxInt64
}

func (c *SystemClock) Step(offset time.Duration) {
	ts, err := unix.TimeToTimespec(c.Now().Add(offset))
	if err != nil {
		panic(fmt.Sprintf("%s unix.TimeToTimespec failed: %v", sysClockLogPrefix, err))
	}
	_, _, errno := unix.Syscall(unix.SYS_CLOCK_SETTIME, uintptr(unix.CLOCK_REALTIME), uintptr(unsafe.Pointer(&ts)), 0)
	if errno != 0 {
		panic(fmt.Sprintf("%s unix.ClockSettime failed: %v", sysClockLogPrefix, errno))
	}
}

func (c *SystemClock) Adjust(offset, duration time.Duration) {
	log.Printf("%s core.SystemClock.Adjust(%v, %v)", sysClockLogPrefix, offset, duration)
}

func (c SystemClock) Sleep(duration time.Duration) {
	log.Printf("%s core.SystemClock.Sleep(%v)", sysClockLogPrefix, duration)
	time.Sleep(duration)
}
