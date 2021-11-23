package drivers

// References:
// http://doc.ntp.org/current-stable/drivers/driver28.html

import (
	"unsafe"

	"fmt"
	"log"
	"os"
	"time"

	"golang.org/x/sys/unix"
)

var (
	shmLog = log.New(os.Stderr, "[etd/shm] ", log.LstdFlags)

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

func initSHM() error {
	var key int = 0x4e545030
	var size int = 96 /* sizeof(struct shmTime) */
	var flags int = 01000 /* IPC_CREAT */ | 0666
	id, _, errno := unix.Syscall(unix.SYS_SHMGET, uintptr(key), uintptr(size), uintptr(flags))
	if int(id) < 0 {
		if int(id) != -1 {
			panic(fmt.Errorf("Syscall shmget returned invalid value: %v", id))
		}
		shmLog.Printf("Syscall shmget failed: %d", errno)
		return errno
	}
	addr, _, errno := unix.Syscall(unix.SYS_SHMAT, id, uintptr(0), uintptr(0))
	if int(addr) == -1 {
		shmLog.Printf("Syscall shmat failed: %d", errno)
		return errno
	}

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

func StoreSHMClockSample(refTime, sysTime time.Time) error {
	if !shmInitialized {
		err := initSHM()
		if err != nil {
			return err
		}
	}

	*shmTimeMode = 0
	*shmTimeClockTimeStampSec = refTime.Unix()
	*shmTimeClockTimeStampUSec = int32(refTime.Nanosecond() / 1e3)
	*shmTimeReceiveTimeStampSec = sysTime.Unix()
	*shmTimeReceiveTimeStampUSec = int32(sysTime.Nanosecond() / 1e3)
	*shmTimeLeap = 0
	*shmTimePrecision = 0
	*shmTimeNSamples = 0
	*shmTimeClockTimeStampNSec = uint32(refTime.Nanosecond())
	*shmTimeReceiveTimeStampNSec = uint32(sysTime.Nanosecond())

	*shmTimeCount++
	*shmTimeValid = 1

	return nil
}
