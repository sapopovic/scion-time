// SCION time dispatcher service.

package main

import (
	"os"
	"runtime"
	"runtime/debug"
	"runtime/pprof"

	"golang.org/x/sys/unix"

	"example.com/scion-time/go/realtime/log"
)

func sleep(sec int64) {
	ts := unix.Timespec{Sec: sec}
	unix.Nanosleep(&ts, nil)
}

func logMemStats(fd int) {
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

func logThreadProfile(fd int, p *pprof.Profile) {
	log.WriteString(fd, "Thread Count: ")
	log.WriteUint64(fd, uint64(p.Count()))
	log.WriteLn(fd)
	log.WriteLn(fd)
}

func monitor(fd int, p *pprof.Profile) {
	threadprofile := pprof.Lookup("threadcreate")
	for {
		sleep(3)
		logMemStats(fd)
		logThreadProfile(fd, threadprofile)
	}
}

func main() {
	stderr := int(os.Stderr.Fd())
	threadprof := pprof.Lookup("threadcreate")
	go monitor(stderr, threadprof)
	debug.SetGCPercent(-1)
	select {}
}

// GODEBUG='allocfreetrace=1,sbrk=1' ./stds
