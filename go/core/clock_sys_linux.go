//go:build linux

package core

import (
	"unsafe"

	"fmt"
	"log"
	"math"
	"sync"
	"sync/atomic"
	"time"

	"golang.org/x/sys/unix"

	"example.com/scion-time/go/core/timebase"
	"example.com/scion-time/go/core/timemath"
)

const (
	sysClockLogPrefix = "[core/clock_sys_linux]"

	ADJ_FREQUENCY = 2

	STA_PLL      = 1
	STA_FREQHOLD = 128
)

type adjustment struct {
	timer     int
	done      uint32
	afterFreq float64
}

type SystemClock struct {
	epoch uint64
	mu    sync.Mutex
	adj   *adjustment
}

var _ timebase.LocalClock = (*SystemClock)(nil)

func newTimer(deadline time.Time) int {
	fd, err := unix.TimerfdCreate(unix.CLOCK_REALTIME, unix.TFD_NONBLOCK)
	if err != nil {
		panic(fmt.Sprintf("%s unix.TimerfdCreate failed: %v", sysClockLogPrefix, err))
	}
	ts, err := unix.TimeToTimespec(deadline)
	if err != nil {
		panic(fmt.Sprintf("%s unix.TimeToTimespec failed: %v", sysClockLogPrefix, err))
	}
	err = unix.TimerfdSettime(fd, unix.TFD_TIMER_ABSTIME, &unix.ItimerSpec{Value: ts}, nil /* oldValue */)
	if err != nil {
		panic(fmt.Sprintf("%s unix.TimerfdSettime failed: %v", sysClockLogPrefix, err))
	}
	return fd
}

func awaitTimer(fd int) {
	pollFds := []unix.PollFd{{Fd: int32(fd), Events: unix.POLLIN}}
	for {
		n, err := unix.Poll(pollFds, -1 /* timeout */)
		if err == unix.EINTR {
			continue
		}
		if err != nil || n != 1 || pollFds[0].Revents != unix.POLLIN {
			panic(fmt.Sprintf("%s unix.Poll failed: %v", sysClockLogPrefix, err))
		}
		break
	}
}

func setFrequency(frequency float64) {
	log.Printf("%s set frequency: %v", sysClockLogPrefix, frequency)
	tx := unix.Timex{
		Modes:  ADJ_FREQUENCY,
		Freq:   int64(math.Floor(frequency * 65536 * 1e6)),
		Status: STA_PLL | STA_FREQHOLD,
	}
	_, err := unix.Adjtimex(&tx)
	if err != nil {
		panic(fmt.Sprintf("%s unix.Adjtimex failed: %v", sysClockLogPrefix, err))
	}
}

func (c *SystemClock) Epoch() uint64 {
	return c.epoch
}

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
	log.Printf("%s set time: %v", sysClockLogPrefix, offset)
	ts, err := unix.TimeToTimespec(c.Now().Add(offset))
	if err != nil {
		panic(fmt.Sprintf("%s unix.TimeToTimespec failed: %v", sysClockLogPrefix, err))
	}
	_, _, errno := unix.Syscall(unix.SYS_CLOCK_SETTIME, uintptr(unix.CLOCK_REALTIME), uintptr(unsafe.Pointer(&ts)), 0)
	if errno != 0 {
		panic(fmt.Sprintf("%s unix.ClockSettime failed: %v", sysClockLogPrefix, errno))
	}
	if c.epoch == math.MaxUint64 {
		panic(fmt.Sprintf("%s epoch overflow", sysClockLogPrefix))
	}
	c.epoch++
}

func (c *SystemClock) Adjust(offset, duration time.Duration, frequency float64) {
	c.mu.Lock()
	c.mu.Unlock()
	if duration < 0 {
		panic(fmt.Sprintf("%s invalid duration value", sysClockLogPrefix))
	}
	duration = duration / time.Second * time.Second
	if duration == 0 {
		duration = time.Second
	}
	if c.adj != nil {
		atomic.StoreUint32(&c.adj.done, 1)
		_ = unix.Close(c.adj.timer)
	}
	setFrequency(frequency + timemath.Seconds(offset)/timemath.Seconds(duration))
	c.adj = &adjustment{
		timer:     newTimer(c.Now().Add(duration)),
		afterFreq: frequency,
	}
	go func(adj *adjustment) {
		awaitTimer(adj.timer)
		if atomic.CompareAndSwapUint32(&adj.done, 0, 1) {
			setFrequency(adj.afterFreq)
		}
	}(c.adj)
}

func (c SystemClock) Sleep(duration time.Duration) {
	log.Printf("%s sleep: %v", sysClockLogPrefix, duration)
	deadline := c.Now().Add(duration)
	awaitTimer(newTimer(deadline))
}
