package shm

// Reference: https://www.ntp.org/documentation/drivers/driver28

import (
	"unsafe"

	"golang.org/x/sys/unix"
)

type segment struct {
	initialized bool

	timeMode                 *int32
	timeCount                *int32
	timeClockTimeStampSec    *int64
	timeClockTimeStampUSec   *int32
	timeReceiveTimeStampSec  *int64
	timeReceiveTimeStampUSec *int32
	timeLeap                 *int32
	timePrecision            *int32
	timeNSamples             *int32
	timeValid                *int32
	timeClockTimeStampNSec   *uint32
	timeReceiveTimeStampNSec *uint32
}

func initSegment(shm *segment, unit int) error {
	if shm.initialized {
		panic("SHM already initialized")
	}

	var key int = 0x4e545030 + unit
	var size int = 96 /* sizeof(struct shmTime) */
	var flags int = 01000 /* IPC_CREAT */ | 0600
	id, _, errno := unix.Syscall(unix.SYS_SHMGET, uintptr(key), uintptr(size), uintptr(flags))
	if int(id) < 0 {
		if int(id) != -1 {
			panic("shmget returned invalid value")
		}
		return errno
	}
	addr, _, errno := unix.Syscall(unix.SYS_SHMAT, id, uintptr(0), uintptr(0))
	if int(addr) == -1 {
		return errno
	}

	// go vet warns about possible misuse of unsafe.Pointer in the following
	// assignments. However, this seems to be in accordance with case (3) in
	// https://pkg.go.dev/unsafe#Pointer. Maybe the follwing accepted proposal
	// will help here: https://github.com/golang/go/issues/58625
	// Another possible workourund would be to use expressions of the form
	// (unsafe.Add(*(*unsafe.Pointer)(unsafe.Pointer(&addr)), ...))
	shm.timeMode = (*int32)(unsafe.Pointer(addr +
		0 /* offsetof(struct shmTime, mode) */))
	shm.timeCount = (*int32)(unsafe.Pointer(addr +
		4 /* offsetof(struct shmTime, count) */))
	shm.timeClockTimeStampSec = (*int64)(unsafe.Pointer(addr +
		8 /* offsetof(struct shmTime, clockTimeStampSec) */))
	shm.timeClockTimeStampUSec = (*int32)(unsafe.Pointer(addr +
		16 /* offsetof(struct shmTime, clockTimeStampUSec) */))
	shm.timeReceiveTimeStampSec = (*int64)(unsafe.Pointer(addr +
		24 /* offsetof(struct shmTime, receiveTimeStampSec) */))
	shm.timeReceiveTimeStampUSec = (*int32)(unsafe.Pointer(addr +
		32 /* offsetof(struct shmTime, receiveTimeStampUSec) */))
	shm.timeLeap = (*int32)(unsafe.Pointer(addr +
		36 /* offsetof(struct shmTime, leap) */))
	shm.timePrecision = (*int32)(unsafe.Pointer(addr +
		40 /* offsetof(struct shmTime, precision) */))
	shm.timeNSamples = (*int32)(unsafe.Pointer(addr +
		44 /* offsetof(struct shmTime, nsamples) */))
	shm.timeValid = (*int32)(unsafe.Pointer(addr +
		48 /* offsetof(struct shmTime, valid) */))
	shm.timeClockTimeStampNSec = (*uint32)(unsafe.Pointer(addr +
		52 /* offsetof(struct shmTime, clockTimeStampNSec) */))
	shm.timeReceiveTimeStampNSec = (*uint32)(unsafe.Pointer(addr +
		56 /* offsetof(struct shmTime, receiveTimeStampNSec) */))

	shm.initialized = true

	return nil
}
