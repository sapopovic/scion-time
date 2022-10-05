//go:build linux

package core

// Based on Ntimed by Poul-Henning Kamp, https://github.com/bsdphk/Ntimed

import (
	"unsafe"

	"fmt"
	"log"
	"math"
	"sync"
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
	clock     *SystemClock
	duration  time.Duration
	afterFreq float64
}

type SystemClock struct {
	mu         sync.Mutex
	epoch      uint64
	adjustment *adjustment
}

var _ timebase.LocalClock = (*SystemClock)(nil)

func now() time.Time {
	var ts unix.Timespec
	err := unix.ClockGettime(unix.CLOCK_REALTIME, &ts)
	if err != nil {
		panic(fmt.Sprintf("%s unix.ClockGettime failed: %v", sysClockLogPrefix, err))
	}
	return time.Unix(ts.Unix()).UTC()
}

func sleep(duration time.Duration) {
	fd, err := unix.TimerfdCreate(unix.CLOCK_REALTIME, unix.TFD_NONBLOCK)
	if err != nil {
		panic(fmt.Sprintf("%s unix.TimerfdCreate failed: %v", sysClockLogPrefix, err))
	}
	ts, err := unix.TimeToTimespec(now().Add(duration))
	if err != nil {
		panic(fmt.Sprintf("%s unix.TimeToTimespec failed: %v", sysClockLogPrefix, err))
	}
	err = unix.TimerfdSettime(fd, unix.TFD_TIMER_ABSTIME, &unix.ItimerSpec{Value: ts}, nil /* oldValue */)
	if err != nil {
		panic(fmt.Sprintf("%s unix.TimerfdSettime failed: %v", sysClockLogPrefix, err))
	}
	if fd < math.MinInt32 || math.MaxInt32 < fd {
		panic(fmt.Sprintf("%s unexpected fd value", sysClockLogPrefix))
	}
	pollFds := []unix.PollFd{
		{Fd: int32(fd), Events: unix.POLLIN},
	}
	for {
		_, err := unix.Poll(pollFds, -1 /* timeout */)
		if err == unix.EINTR {
			continue
		}
		if err != nil {
			panic(fmt.Sprintf("%s unix.Poll failed: %v", sysClockLogPrefix, err))
		}
		break
	}
	_ = unix.Close(fd)
}

func setTime(offset time.Duration) {
	log.Printf("%s set time: %v", sysClockLogPrefix, offset)
	ts, err := unix.TimeToTimespec(now().Add(offset))
	if err != nil {
		panic(fmt.Sprintf("%s unix.TimeToTimespec failed: %v", sysClockLogPrefix, err))
	}
	_, _, errno := unix.Syscall(unix.SYS_CLOCK_SETTIME, uintptr(unix.CLOCK_REALTIME), uintptr(unsafe.Pointer(&ts)), 0)
	if errno != 0 {
		panic(fmt.Sprintf("%s unix.ClockSettime failed: %v", sysClockLogPrefix, errno))
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
	c.mu.Lock()
	c.mu.Unlock()
	return c.epoch
}

func (c *SystemClock) Now() time.Time {
	return now()
}

func (c *SystemClock) MaxDrift(duration time.Duration) time.Duration {
	return math.MaxInt64
}

func (c *SystemClock) Step(offset time.Duration) {
	c.mu.Lock()
	c.mu.Unlock()
	if c.adjustment != nil {
		c.adjustment = nil
	}
	setTime(offset)
	if c.epoch == math.MaxUint64 {
		panic(fmt.Sprintf("%s epoch overflow", sysClockLogPrefix))
	}
	c.epoch++
}

func (c *SystemClock) Adjust(offset, duration time.Duration, frequency float64) {
	c.mu.Lock()
	c.mu.Unlock()
	if c.adjustment != nil {
		c.adjustment = nil
	}
	if duration < 0 {
		panic(fmt.Sprintf("%s invalid duration value", sysClockLogPrefix))
	}
	duration = duration / time.Second * time.Second
	if duration == 0 {
		duration = time.Second
	}
	setFrequency(frequency + timemath.Seconds(offset)/timemath.Seconds(duration))
	c.adjustment = &adjustment{
		clock:     c,
		duration:  duration,
		afterFreq: frequency,
	}
	go func(adj *adjustment) {
		sleep(adj.duration)
		adj.clock.mu.Lock()
		defer adj.clock.mu.Unlock()
		if adj == adj.clock.adjustment {
			setFrequency(adj.afterFreq)
		}
	}(c.adjustment)
}

func (c SystemClock) Sleep(duration time.Duration) {
	log.Printf("%s sleep: %v", sysClockLogPrefix, duration)
	if duration < 0 {
		panic(fmt.Sprintf("%s invalid duration value", sysClockLogPrefix))
	}
	sleep(duration)
}
