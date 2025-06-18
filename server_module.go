package main

import (
	"gosocket/gamepacket"
	"net"
)

type ServerModule interface {
	CanHandle(*gamepacket.GamePacket) bool
	Handle(conn net.PacketConn, addr net.Addr, pkt *gamepacket.GamePacket)
}
