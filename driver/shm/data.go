package shm

// Reference: https://www.ntp.org/documentation/drivers/driver28

import (
	"unsafe"

	"go.uber.org/zap"

	"golang.org/x/sys/unix"
)

var (
	shmInitialized bool

	shmTimeMode                 *int32
	shmTimeCount                *int32
	shmTimeClockTimeStampSec    *int64
	shmTimeClockTimeStampUSec   *int32
	shmTimeReceiveTimeStampSec  *int64
	shmTimeReceiveTimeStampUSec *int32
	shmTimeLeap                 *int32
	shmTimePrecision            *int32
	shmTimeNSamples             *int32
	shmTimeValid                *int32
	shmTimeClockTimeStampNSec   *uint32
	shmTimeReceiveTimeStampNSec *uint32
)

func initSHM(log *zap.Logger, unit int) error {
	if shmInitialized {
		panic("SHM already initialized")
	}

	var key int = 0x4e545030 + unit
	var size int = 96 /* sizeof(struct shmTime) */
	var flags int = 01000 /* IPC_CREAT */ | 0600
	id, _, errno := unix.Syscall(unix.SYS_SHMGET, uintptr(key), uintptr(size), uintptr(flags))
	if int(id) < 0 {
		if int(id) != -1 {
			log.Fatal("shmget returned invalid value", zap.Uintptr("id", id))
		}
		log.Error("shmget failed", zap.Error(errno))
		return errno
	}
	addr, _, errno := unix.Syscall(unix.SYS_SHMAT, id, uintptr(0), uintptr(0))
	if int(addr) == -1 {
		log.Error("shmat failed", zap.Error(errno))
		return errno
	}

	// go vet warns about possible misuse of unsafe.Pointer in the following
	// assignments. However, this seems to be in accordance with case (3) in
	// https://pkg.go.dev/unsafe#Pointer. Maybe the follwing accepted proposal
	// will help here: https://github.com/golang/go/issues/58625
	// Another possible workourund would be to use expressions of the form
	// (unsafe.Add(*(*unsafe.Pointer)(unsafe.Pointer(&addr)), ...))
	shmTimeMode = (*int32)(unsafe.Pointer(addr +
		0 /* offsetof(struct shmTime, mode) */))
	shmTimeCount = (*int32)(unsafe.Pointer(addr +
		4 /* offsetof(struct shmTime, count) */))
	shmTimeClockTimeStampSec = (*int64)(unsafe.Pointer(addr +
		8 /* offsetof(struct shmTime, clockTimeStampSec) */))
	shmTimeClockTimeStampUSec = (*int32)(unsafe.Pointer(addr +
		16 /* offsetof(struct shmTime, clockTimeStampUSec) */))
	shmTimeReceiveTimeStampSec = (*int64)(unsafe.Pointer(addr +
		24 /* offsetof(struct shmTime, receiveTimeStampSec) */))
	shmTimeReceiveTimeStampUSec = (*int32)(unsafe.Pointer(addr +
		32 /* offsetof(struct shmTime, receiveTimeStampUSec) */))
	shmTimeLeap = (*int32)(unsafe.Pointer(addr +
		36 /* offsetof(struct shmTime, leap) */))
	shmTimePrecision = (*int32)(unsafe.Pointer(addr +
		40 /* offsetof(struct shmTime, precision) */))
	shmTimeNSamples = (*int32)(unsafe.Pointer(addr +
		44 /* offsetof(struct shmTime, nsamples) */))
	shmTimeValid = (*int32)(unsafe.Pointer(addr +
		48 /* offsetof(struct shmTime, valid) */))
	shmTimeClockTimeStampNSec = (*uint32)(unsafe.Pointer(addr +
		52 /* offsetof(struct shmTime, clockTimeStampNSec) */))
	shmTimeReceiveTimeStampNSec = (*uint32)(unsafe.Pointer(addr +
		56 /* offsetof(struct shmTime, receiveTimeStampNSec) */))

	shmInitialized = true

	return nil
}
