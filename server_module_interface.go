package main

import (
	"fmt"
	"gosocket/gamepacket"
	"net"
)

type ServerModule interface {
	CanHandle(*gamepacket.GamePacket) bool
	Handle(conn net.PacketConn, addr net.Addr, pkt *gamepacket.GamePacket)
}

var modules []ServerModule

func init() {
	movementModule := NewMovementModule()

	modules = []ServerModule{
		NewConnectionModule(),
		NewLobbyModule(),
		movementModule,
	}
	movementModule.StartWorkers()
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

func shutdownModules() {
	fmt.Println("🛑 Shutting down all modules...")

	for _, module := range modules {
		if movementModule, ok := module.(*MovementModule); ok {
			fmt.Println("🛑 Stopping movement workers...")
			movementModule.StopWorkers()
		}
	}

	fmt.Println("✅ All modules shut down successfully")
}
