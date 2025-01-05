package csptp

const (
	EventPortIP      = 319   // Sync
	EventPortSCION   = 10319 // Sync
	GeneralPortIP    = 320   // Follow Up
	GeneralPortSCION = 10320 // Follow Up

	SdoID = 0

	MsgTypeSync     = 0
	MsgTypeFollowUp = 8

	VersionMin     = 1
	VersionMax     = 0x12
	VersionDefault = 0x12

	Domain = 0
)

type PortID struct {
	ClockID uint64
	Port    uint16
}

type Timestamp struct {
	Seconds     [6]uint8
	Nanoseconds uint32
}

type Packet struct {
	SdoIDMsgType    uint8
	Version         uint8
	MsgLen          uint16
	Domain          uint8
	Flags           uint16
	Correction      int64
	MsgTypeSpecific uint32
	PortID          PortID
	SeqID           uint16
	MsgCtrl         uint8
	LogMsgPeriod    int8
	Timestamp       Timestamp
}

func DecodePacket(pkt *Packet, b []byte) error {
	return nil
}

func EncodePacket(b *[]byte, pkt *Packet) {}
