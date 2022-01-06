package udp

import "golang.org/x/sys/unix"

const (
	SCM_TIMESTAMP   = unix.SCM_TIMESTAMP
	SCM_TIMESTAMPNS = unix.SCM_TIMESTAMPNS
	SO_TIMESTAMP    = unix.SO_TIMESTAMP
	SO_TIMESTAMPNS  = unix.SO_TIMESTAMPNS
)
