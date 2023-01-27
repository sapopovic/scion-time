package scion

import (
	"github.com/scionproto/scion/pkg/drkey"
)

const (
	DRKeyTypeHostHost          = 1
	DRKeyDirectionReceiverSide = 1
	DRKeyEpochLater            = 0
	DRKeyProtoIdTS             = drkey.SCMP

	PacketAuthOptDataLen = 12 /* len(metadata) */ + 16 /* len(MAC) */

	PacketAuthClientSPI = uint32(DRKeyTypeHostHost) << 18 |
		uint32(DRKeyDirectionReceiverSide) << 17 |
		uint32(DRKeyEpochLater) << 16 |
		uint32(DRKeyProtoIdTS)
	PacketAuthAlgorithm = uint8(0) // AES-CMAC
)
