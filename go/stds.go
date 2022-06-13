// SCION time dispatcher service.

package main

import (
	"os"
	"runtime"
	"runtime/pprof"

	"golang.org/x/sys/unix"

	"example.com/scion-time/go/realtime/log"
)

func sleep(sec int64) {
	ts := unix.Timespec{Sec: sec}
	unix.Nanosleep(&ts, nil)
}

func newSemaphore(initval uint) int {
	fd, err := unix.Eventfd(initval, unix.EFD_NONBLOCK|unix.EFD_SEMAPHORE)
	if err != nil {
		panic("newSemaphore: unix.Eventfd failed")
	}
	return fd
}

func pollSemaphore(fd int) bool {
	val := []byte{0, 0, 0, 0, 0, 0, 0, 0}
	for {
		n, err := unix.Read(fd, val)
		if err == unix.EINTR {
			continue
		}
		if err == unix.EAGAIN || err == unix.EWOULDBLOCK {
			return false
		}
		if err != nil || n != 8 ||
			val[0] != 1 || val[1] != 0 || val[2] != 0 || val[3] != 0 ||
			val[4] != 0 || val[5] != 0 || val[6] != 0 || val[7] != 0 {
			panic("pollSemaphore: unix.Read failed")
		}
		return true
	}
}

func signalSemaphore(fd int) {
	val := []byte{1, 0, 0, 0, 0, 0, 0, 0}
	for {
		n, err := unix.Write(fd, val)
		if err == unix.EINTR {
			continue
		}
		if err != nil || n != len(val) {
			panic("signalSemaphore: unix.Write failed")
		}
	}
}

func awaitSemaphore(fd int) {
	ok := pollSemaphore(fd)
	for !ok {
		events := []unix.PollFd{
			{
				Fd:     int32(fd),
				Events: unix.POLLIN,
			},
		}
		for {
			n, err := unix.Poll(events, -1)
			if err == unix.EINTR || err == unix.EAGAIN {
				continue
			}
			if err != nil || n != 1 || events[0].Revents != unix.POLLIN {
				panic("awaitSemaphore: unix.Poll failed")
			}
			break
		}
		ok = pollSemaphore(fd)
	}
}

func logMemStats(fd, sem int) {
	awaitSemaphore(sem)
	defer signalSemaphore(sem)
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

func logThreadProfile(fd, sem int, p *pprof.Profile) {
	awaitSemaphore(sem)
	defer signalSemaphore(sem)
	log.WriteString(fd, "Thread Count: ")
	log.WriteUint64(fd, uint64(p.Count()))
	log.WriteLn(fd)
	log.WriteLn(fd)
}

func monitor(logFd, logSem int, p *pprof.Profile) {
	threadprofile := pprof.Lookup("threadcreate")
	for {
		sleep(15)
		logMemStats(logFd, logSem)
		logThreadProfile(logFd, logSem, threadprofile)
	}
}

func run(id, logFd, semFd int) {
	awaitSemaphore(semFd)
	log.WriteString(logFd, "running: ")
	log.WriteUint64(logFd, uint64(id))
	log.WriteLn(logFd)
	signalSemaphore(semFd)

	select {}
}

func main() {
	stdout := int(os.Stdout.Fd())
	stderr := int(os.Stderr.Fd())

	logSem := newSemaphore(1)

	threadprof := pprof.Lookup("threadcreate")
	go monitor(stderr, logSem, threadprof)

	for i := 0; i != 8; i++ {
		go run(i, stdout, logSem)
	}

	select {}
}

// GOGC=off GODEBUG='allocfreetrace=1,sbrk=1' ./stds
