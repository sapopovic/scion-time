package udp

import "golang.org/x/sys/unix"

const (
	scm_timestamp   = unix.SCM_TIMESTAMP
	scm_timestampns = unix.SCM_TIMESTAMPNS
	so_timestamp    = unix.SO_TIMESTAMP
	so_timestampns  = unix.SO_TIMESTAMPNS
)
