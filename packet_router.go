package main

import (
	"fmt"
	"gosocket/gamepacket"
	"net"
)

var modules []ServerModule

func init() {
	modules = []ServerModule{
		NewConnectionModule(),
		NewLobbyModule(),
		NewChatModule(),
	}
}

func dispatchPacket(conn net.PacketConn, addr net.Addr, pkt *gamepacket.GamePacket) {
	for _, m := range modules {
		if m.CanHandle(pkt) {
			m.Handle(conn, addr, pkt)
			return
		}
	}
	fmt.Println("⚠️ No module handled packet")
}
