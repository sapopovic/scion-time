package scion

import (
	"github.com/scionproto/scion/pkg/drkey"
)

const (
	DRKeyTypeHostHost          = 1
	DRKeyDirectionSenderSide   = 0
	DRKeyDirectionReceiverSide = 1
	DRKeyProtoIdTS             = drkey.SCMP

	PacketAuthMetadataLen = 12
	PacketAuthMACLen      = 16
	PacketAuthOptDataLen  = PacketAuthMetadataLen + PacketAuthMACLen

	PacketAuthClientSPI = uint32(DRKeyTypeHostHost)<<17 |
		uint32(DRKeyDirectionReceiverSide)<<16 |
		uint32(DRKeyProtoIdTS)
	PacketAuthServerSPI = uint32(DRKeyTypeHostHost)<<17 |
		uint32(DRKeyDirectionSenderSide)<<16 |
		uint32(DRKeyProtoIdTS)
	PacketAuthAlgorithm = uint8(0) // AES-CMAC
)
