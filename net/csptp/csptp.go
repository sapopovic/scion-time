package csptp

type Packet struct {}

func DecodePacket(pkt *Packet, b []byte) error {
	return nil
}

func EncodePacket(b *[]byte, pkt *Packet) {}
