// SCION time dispatcher service.

package main

import (
	"os"
	"runtime"
	"runtime/pprof"

	"golang.org/x/sys/unix"

	"example.com/scion-time/go/realtime/log"
	"example.com/scion-time/go/realtime/sync"
)

type pool struct {
	nonempty *sync.Semaphore
	nonfull *sync.Semaphore
	bufSem *sync.Semaphore
	bufCtx sync.Context
	buf []any
}

func newPool(cap int) *pool {
	if cap <= 0 {
		panic("capacity must be greater than 0")
	}
	p := &pool{
		nonempty: sync.NewSemaphore(uint(cap)),
		nonfull: sync.NewSemaphore(0),
		bufSem: sync.NewSemaphore(1),
		bufCtx: sync.Context{},
		buf: make([]any, cap),
	}
	return p
}

func sleep(sec, nsec int64) {
	ts := unix.Timespec{Sec: sec, Nsec: nsec}
	unix.Nanosleep(&ts, nil)
}

func await(s *sync.Semaphore) {
	ok := s.Acquire()
	var events [1]unix.PollFd
	for !ok {
		events[0] = unix.PollFd{
			Fd:     int32(s.Fd()),
			Events: unix.POLLIN,
		}
		for {
			n, err := unix.Poll(events[:], -1)
			if err == unix.EINTR || err == unix.EAGAIN {
				continue
			}
			if err != nil || n != 1 || events[0].Revents != unix.POLLIN {
				panic("awaitSemaphore: unix.Poll failed")
			}
			break
		}
		ok = s.Acquire()
	}
}

func logMemStats(fd int, sem *sync.Semaphore) {
	await(sem)
	defer sem.Release()
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	log.WriteString(fd, "TotalAlloc: ")
	log.WriteUint64(fd, uint64(m.TotalAlloc))
	log.WriteLn(fd)
	log.WriteString(fd, "Mallocs: ")
	log.WriteUint64(fd, uint64(m.Mallocs))
	log.WriteLn(fd)
	log.WriteString(fd, "Frees: ")
	log.WriteUint64(fd, uint64(m.Frees))
	log.WriteLn(fd)
	log.WriteString(fd, "NumGC: ")
	log.WriteUint64(fd, uint64(m.NumGC))
	log.WriteLn(fd)
	log.WriteLn(fd)
}

func logThreadProfile(fd int, sem *sync.Semaphore, p *pprof.Profile) {
	await(sem)
	defer sem.Release()
	log.WriteString(fd, "Thread Count: ")
	log.WriteUint64(fd, uint64(p.Count()))
	log.WriteLn(fd)
	log.WriteLn(fd)
}

func monitor(logFd int, logSem *sync.Semaphore, p *pprof.Profile) {
	await(logSem)
	log.WriteString(logFd, "running: monitor")
	log.WriteLn(logFd)
	logSem.Release()

	threadprofile := pprof.Lookup("threadcreate")
	for {
		sleep(15, 0)
		logMemStats(logFd, logSem)
		logThreadProfile(logFd, logSem, threadprofile)
	}
}

func run(id, logFd int, logSem *sync.Semaphore, p *pool) {
	await(logSem)
	log.WriteString(logFd, "running: ")
	log.WriteUint64(logFd, uint64(id))
	log.WriteLn(logFd)
	logSem.Release()

	for i := 0; i != 5_000_000; i++ {
		await(p.nonempty)
		await(p.bufSem)
		p.bufCtx.Open()

		if i % 100_000 == 0 {
			await(logSem)
			runtime.LockOSThread()
			log.WriteString(logFd, "consuming: ")
			log.WriteUint64(logFd, uint64(id))
			log.WriteString(logFd, ", ")
			log.WriteUint64(logFd, uint64(i))
			log.WriteLn(logFd)
			runtime.UnlockOSThread()
			logSem.Release()
		}

		p.bufCtx.Close()
		p.bufSem.Release()
		p.nonfull.Release()

		sleep(0, 0)

		await(p.nonfull)
		await(p.bufSem)
		p.bufCtx.Open()

		if i % 100_000 == 0 {
			await(logSem)
			runtime.LockOSThread()
			log.WriteString(logFd, "producing: ")
			log.WriteUint64(logFd, uint64(id))
			log.WriteString(logFd, ", ")
			log.WriteUint64(logFd, uint64(i))
			log.WriteLn(logFd)
			runtime.UnlockOSThread()
			logSem.Release()
		}

		p.bufCtx.Close()
		p.bufSem.Release()
		p.nonempty.Release()

		sleep(0, 0)
	}

	await(logSem)
	log.WriteString(logFd, "done producing: ")
	log.WriteUint64(logFd, uint64(id))
	log.WriteLn(logFd)
	logSem.Release()

	pollFd, err := unix.EpollCreate1(0)
	if err != nil {
		panic("run: unix.EpollCreate1 failed")
	}
	var events [16]unix.EpollEvent
	for {
		_, err := unix.EpollWait(pollFd, events[:], -1)
		if err == unix.EINTR {
			continue
		}
	}
}

func main() {
	stdout := int(os.Stdout.Fd())
	stderr := int(os.Stderr.Fd())

	logSem := sync.NewSemaphore(1)

	threadprof := pprof.Lookup("threadcreate")
	go monitor(stderr, logSem, threadprof)

	pool := newPool(4)

	for i := 0; i != 64; i++ {
		go run(i, stdout, logSem, pool)
	}

	await(sync.NewSemaphore(0))
}

// GOGC=off GODEBUG='allocfreetrace=1,sbrk=1' ./stds
