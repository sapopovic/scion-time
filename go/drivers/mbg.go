package drivers

// References:
// https://kb.meinbergglobal.com/kb/driver_software/meinberg_sdks/meinberg_driver_and_api_concepts
// https://kb.meinbergglobal.com/mbglib-api/

import (
	"unsafe"

	"encoding/binary"
	"log"
	"os"
	"time"

	"golang.org/x/sys/unix"
)

const (
	// See https://man7.org/linux/man-pages/man2/ioctl.2.html#NOTES

	ioctlWrite = 1
	ioctlRead  = 2

	ioctlDirBits  = 2
	ioctlSizeBits = 14
	ioctlTypeBits = 8
	ioctlSNBits   = 8

	ioctlDirMask  = (1 << ioctlDirBits) - 1
	ioctlSizeMask = (1 << ioctlSizeBits) - 1
	ioctlTypeMask = (1 << ioctlTypeBits) - 1
	ioctlSNMask   = (1 << ioctlSNBits) - 1

	ioctlSNShift   = 0
	ioctlTypeShift = ioctlSNShift + ioctlSNBits
	ioctlSizeShift = ioctlTypeShift + ioctlTypeBits
	ioctlDirShift  = ioctlSizeShift + ioctlSizeBits
)

var mbgLog = log.New(os.Stderr, "[ets/mbg] ", log.LstdFlags)

func ioctlRequest(d, s, t, n int) uint {
	// See https://man7.org/linux/man-pages/man2/ioctl.2.html#NOTES

	return (uint(d&ioctlDirMask) << ioctlDirShift) |
		(uint(s&ioctlSizeMask) << ioctlSizeShift) |
		(uint(t&ioctlTypeMask) << ioctlTypeShift) |
		(uint(n&ioctlSNMask) << ioctlSNShift)
}

func nanoseconds(frac uint32) int64 {
	// Binary fractions to nanoseconds:
	// nanoseconds(0x00000000) == 0
	// nanoseconds(0x80000000) == 500000000
	// nanoseconds(0xffffffff) == 999999999

	return int64((uint64(frac) * uint64(time.Second)) / (1 << 32))
}

func FetchMBGTime(dev string) (refTime time.Time, sysTime time.Time, err error) {
	fd, err := unix.Open(dev, unix.O_RDWR, 0)
	if err != nil {
		mbgLog.Printf("Failed to open %s: %v", dev, err)
		return time.Time{}, time.Time{}, err 
	}
	defer func() {
		err = unix.Close(fd)
		if err != nil {
			mbgLog.Printf("Failed to close %s: %v", dev, err)
		}
	}()

	featureType := uint32(2 /* PCPS */)
	featureNumber := uint32(6 /* HAS_HR_TIME */)

	featureData := make([]byte, 4+4)
	binary.LittleEndian.PutUint32(featureData[0:], featureType)
	binary.LittleEndian.PutUint32(featureData[4:], featureNumber)

	_, _, errno := unix.Syscall(unix.SYS_IOCTL, uintptr(fd),
		uintptr(ioctlRequest(ioctlWrite, len(featureData), 'M', 0xa4)),
		uintptr(unsafe.Pointer(&featureData[0])))
	if errno != 0 {
		mbgLog.Printf("Failed to ioctl %s (features) or HR time not supported: %d", dev, errno)
		return time.Time{}, time.Time{}, errno
	}

	cycleFrequencyData := make([]byte, 8)
	_, _, errno = unix.Syscall(unix.SYS_IOCTL, uintptr(fd),
		uintptr(ioctlRequest(ioctlRead, len(cycleFrequencyData), 'M', 0x68)),
		uintptr(unsafe.Pointer(&cycleFrequencyData[0])))
	if errno != 0 {
		mbgLog.Printf("Failed to ioctl %s (cycle frequency): %d", dev, errno)
		return time.Time{}, time.Time{}, errno
	}

	cycleFrequency := binary.LittleEndian.Uint64(cycleFrequencyData[0:])

	timeData := make([]byte, 8+4+4+4+2+1+8+8+8+8)

	_, _, errno = unix.Syscall(unix.SYS_IOCTL, uintptr(fd),
		uintptr(ioctlRequest(ioctlRead, len(timeData), 'M', 0x80)),
		uintptr(unsafe.Pointer(&timeData[0])))
	if errno != 0 {
		mbgLog.Printf("Failed to ioctl %s (time): %d", dev, errno)
		return time.Time{}, time.Time{}, errno
	}

	refTimeCycles := int64(binary.LittleEndian.Uint64(timeData[0:]))
	refTimeSeconds := int64(binary.LittleEndian.Uint32(timeData[8:]))
	refTimeFractions := uint32(binary.LittleEndian.Uint32(timeData[12:]))
	refTimeUTCOffset := int32(binary.LittleEndian.Uint32(timeData[16:]))
	refTimeStatus := uint16(binary.LittleEndian.Uint16(timeData[20:]))
	refTimeSignal := uint8(timeData[22])
	sysTimeCyclesBefore := int64(binary.LittleEndian.Uint64(timeData[23:]))
	sysTimeCyclesAfter := int64(binary.LittleEndian.Uint64(timeData[31:]))
	sysTimeSeconds := int64(binary.LittleEndian.Uint64(timeData[39:]))
	sysTimeNanoseconds := int64(binary.LittleEndian.Uint64(timeData[47:]))

	refTime = time.Unix(refTimeSeconds, nanoseconds(refTimeFractions)).UTC()
	sysTime = time.Unix(sysTimeSeconds, sysTimeNanoseconds).UTC()

	mbgLog.Printf("RefTime: %v, UTC offset: %v, status: %v, signal: %v",
		refTime, refTimeUTCOffset, refTimeStatus, refTimeSignal)
	mbgLog.Printf("SysTime: %v, at: %v, latency: %v, frequency: %v",
		sysTime, sysTimeCyclesBefore, refTimeCycles-sysTimeCyclesAfter, cycleFrequency)
	mbgLog.Printf("Offset: %v\n", refTime.Sub(sysTime))

	return refTime, sysTime, nil
}
