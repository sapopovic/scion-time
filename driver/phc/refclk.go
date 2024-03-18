package phc

// Reference: https://github.com/torvalds/linux/blob/master/include/uapi/linux/ptp_clock.h

import (
	"unsafe"

	"context"
	"time"

	"go.uber.org/zap"

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

type ptpClockTime struct {
	sec      int64  /* seconds */
	nsec     uint32 /* nanoseconds */
	reserved uint32
}

const (
	sizeofPTPClockTime = 16 // sizeof(struct ptp_clock_time)

	offsetofPTPClockTimeSec  = 0 // offsetof(struct ptp_clock_time, sec)
	offsetofPTPClockTimeNSec = 8 // offsetof(struct ptp_clock_time, nsec)
)

type ptpSysOffsetPrecise struct {
	device      ptpClockTime
	sysRealTime ptpClockTime
	sysMonoRaw  ptpClockTime
	reserved    [4]uint32 /* Reserved for future use. */
}

const (
	sizeofPTPSysOffsetPrecise = 64 // sizeof(struct ptp_sys_offset_precise)

	offsetofPTPSysOffsetPreciseDevice      = 0  // offsetof(struct ptp_sys_offset_precise, device)
	offsetofPTPSysOffsetPreciseSysRealTime = 16 // offsetof(struct ptp_sys_offset_precise, sys_realtime)
	offsetofPTPSysOffsetPreciseSysMonoRaw  = 32 // offsetof(struct ptp_sys_offset_precise, sys_monoraw)
)

type ReferenceClock struct {
	dev string
}

func init() {
	var t0 ptpClockTime
	if unsafe.Sizeof(t0) != sizeofPTPClockTime ||
		unsafe.Offsetof(t0.sec) != offsetofPTPClockTimeSec ||
		unsafe.Offsetof(t0.nsec) != offsetofPTPClockTimeNSec {
		panic("unexpected memory layout")
	}
	var t1 ptpSysOffsetPrecise
	if unsafe.Sizeof(t1) != sizeofPTPSysOffsetPrecise ||
		unsafe.Offsetof(t1.device) != offsetofPTPSysOffsetPreciseDevice ||
		unsafe.Offsetof(t1.sysRealTime) != offsetofPTPSysOffsetPreciseSysRealTime ||
		unsafe.Offsetof(t1.sysMonoRaw) != offsetofPTPSysOffsetPreciseSysMonoRaw {
		panic("unexpected memory layout")
	}
}

func ioctlRequest(d, s, t, n int) uint {
	// See https://man7.org/linux/man-pages/man2/ioctl.2.html#NOTES

	return (uint(d&ioctlDirMask) << ioctlDirShift) |
		(uint(s&ioctlSizeMask) << ioctlSizeShift) |
		(uint(t&ioctlTypeMask) << ioctlTypeShift) |
		(uint(n&ioctlSNMask) << ioctlSNShift)
}

func NewReferenceClock(dev string) *ReferenceClock {
	return &ReferenceClock{dev: dev}
}

func (c *ReferenceClock) MeasureClockOffset(ctx context.Context, log *zap.Logger) (time.Duration, error) {
	fd, err := unix.Open(c.dev, unix.O_RDWR, 0)
	if err != nil {
		log.Error("unix.Open failed", zap.String("dev", c.dev), zap.Error(err))
		return 0, err
	}
	defer func(log *zap.Logger, dev string) {
		err = unix.Close(fd)
		if err != nil {
			log.Info("unix.Close failed", zap.String("dev", dev), zap.Error(err))
		}
	}(log, c.dev)

	off := ptpSysOffsetPrecise{}
	_, _, errno := unix.Syscall(unix.SYS_IOCTL, uintptr(fd),
		uintptr(ioctlRequest(ioctlRead|ioctlWrite, int(unsafe.Sizeof(off)), '=', 0x8)),
		uintptr(unsafe.Pointer(&off)),
	)
	if errno != 0 {
		log.Error("ioctl failed", zap.String("dev", c.dev), zap.Error(errno))
		return 0, errno
	}

	sysRealTime := time.Unix(off.sysRealTime.sec, int64(off.sysRealTime.nsec)).UTC()
	deviceTime := time.Unix(off.device.sec, int64(off.device.nsec)).UTC()
	offset := deviceTime.Sub(sysRealTime)

	log.Debug("PTP hardware clock sample",
		zap.Time("sysRealTime", sysRealTime),
		zap.Time("deviceTime", deviceTime),
		zap.Duration("offset", offset),
	)

	return offset, nil
}
