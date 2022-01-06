package udp

import "golang.org/x/sys/unix"

const (
	SCM_TIMESTAMP   = unix.SCM_TIMESTAMP
	SCM_TIMESTAMPNS = 0
	SO_TIMESTAMP    = unix.SO_TIMESTAMP
	SO_TIMESTAMPNS  = 0
)
