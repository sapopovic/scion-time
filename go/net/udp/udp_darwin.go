package udp

import "golang.org/x/sys/unix"

const (
	scm_timestamp   = unix.SCM_TIMESTAMP
	scm_timestampns = 0
	so_timestamp    = unix.SO_TIMESTAMP
	so_timestampns  = 0
)
