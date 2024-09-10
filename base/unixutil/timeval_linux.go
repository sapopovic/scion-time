package unixutil

import (
	"golang.org/x/sys/unix"
)

func TimevalFromNsec(nsec int64) unix.Timeval {
	sec := nsec / 1e9
	nsec = nsec % 1e9
	// The field unix.Timeval.Usec must always be non-negative.
	if nsec < 0 {
		sec -= 1
		nsec += 1e9
	}
	return unix.Timeval{
		Sec:  sec,
		Usec: nsec,
	}
}
