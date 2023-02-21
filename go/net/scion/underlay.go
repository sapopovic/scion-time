package scion

const (
	EndhostPort = 30041

	// MTU supported by SCION.
	// It's chosen as a common ethernet jumbo frame size minus IP/UDP headers.
	MTU = 9216 - 20 - 8
)
