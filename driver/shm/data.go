package shm

// Reference: https://www.ntp.org/documentation/drivers/driver28

import (
	"unsafe"

	"golang.org/x/sys/unix"
)

type shmTime struct {
	mode int32
	// 0 - if valid is set:
	//       use values
	//       clear valid
	// 1 - if valid is set:
	//       if count before and after read of data is equal:
	//         use values
	//       clear valid
	count                int32
	clockTimeStampSec    int64
	clockTimeStampUSec   int32
	receiveTimeStampSec  int64
	receiveTimeStampUSec int32
	leap                 int32
	precision            int32
	nSamples             int32
	valid                int32
	clockTimeStampNSec   uint32 // Unsigned ns timestamps
	receiveTimeStampNSec uint32 // Unsigned ns timestamps
	dummy                [8]int32
}

const (
	offsetOfMode                 = 0  // offsetof(struct shmTime, mode)
	offsetOfCount                = 4  // offsetof(struct shmTime, count)
	offsetOfClockTimeStampSec    = 8  // offsetof(struct shmTime, clockTimeStampSec)
	offsetOfClockTimeStampUSec   = 16 // offsetof(struct shmTime, clockTimeStampUSec)
	offsetOfReceiveTimeStampSec  = 24 // offsetof(struct shmTime, receiveTimeStampSec)
	offsetOfReceiveTimeStampUSec = 32 // offsetof(struct shmTime, receiveTimeStampUSec)
	offsetOfLeap                 = 36 // offsetof(struct shmTime, leap)
	offsetOfPrecision            = 40 // offsetof(struct shmTime, precision)
	offsetOfNSamples             = 44 // offsetof(struct shmTime, nsamples)
	offsetOfValid                = 48 // offsetof(struct shmTime, valid)
	offsetOfClockTimeStampNSec   = 52 // offsetof(struct shmTime, clockTimeStampNSec)
	offsetOfReceiveTimeStampNSec = 56 // offsetof(struct shmTime, receiveTimeStampNSec)
)

type segment struct {
	initialized bool
	time        *shmTime
}

func initSegment(shm *segment, unit int) error {
	if shm.initialized {
		panic("SHM already initialized")
	}

	if unsafe.Offsetof(shm.time.mode) != offsetOfMode ||
		unsafe.Offsetof(shm.time.count) != offsetOfCount ||
		unsafe.Offsetof(shm.time.clockTimeStampSec) != offsetOfClockTimeStampSec ||
		unsafe.Offsetof(shm.time.clockTimeStampUSec) != offsetOfClockTimeStampUSec ||
		unsafe.Offsetof(shm.time.receiveTimeStampSec) != offsetOfReceiveTimeStampSec ||
		unsafe.Offsetof(shm.time.receiveTimeStampUSec) != offsetOfReceiveTimeStampUSec ||
		unsafe.Offsetof(shm.time.leap) != offsetOfLeap ||
		unsafe.Offsetof(shm.time.precision) != offsetOfPrecision ||
		unsafe.Offsetof(shm.time.nSamples) != offsetOfNSamples ||
		unsafe.Offsetof(shm.time.valid) != offsetOfValid ||
		unsafe.Offsetof(shm.time.clockTimeStampNSec) != offsetOfClockTimeStampNSec ||
		unsafe.Offsetof(shm.time.receiveTimeStampNSec) != offsetOfReceiveTimeStampNSec {
		panic("unexpected memory layout")
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

	// go vet warns about a possible misuse of unsafe.Pointer in
	// `shm.time = (*shmTime)(unsafe.Pointer(addr))`
	// However, this seems to be in accordance with case (3) in
	// https://pkg.go.dev/unsafe#Pointer.
	// Maybe the follwing accepted proposal will help here:
	// https://github.com/golang/go/issues/58625
	shm.time = (*shmTime)(*(*unsafe.Pointer)(unsafe.Pointer(&addr)))

	shm.initialized = true

	return nil
}
