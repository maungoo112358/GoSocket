package main

import (
	"fmt"
	"gosocket/gamepacket"
	"math/rand"
	"net"
	"time"

	"google.golang.org/protobuf/proto"
)

type MovementModule struct {
	name string
}

func NewMovementModule() *MovementModule {
	return &MovementModule{name: "MovementModule"}
}

func (m *MovementModule) GetName() string {
	return m.name
}

func (m *MovementModule) CanHandle(pkt *gamepacket.GamePacket) bool {
	return pkt.GetClientPosition() != nil
}

func (m *MovementModule) Handle(conn net.PacketConn, addr net.Addr, pkt *gamepacket.GamePacket) {
	clientPos := pkt.GetClientPosition()
	if clientPos == nil {
		return
	}

	// ✅ Add validation
	if !isValidPosition(clientPos.Position) {
		fmt.Printf("⚠️ Invalid position from %s: %.2f,%.2f,%.2f\n",
			clientPos.ClientId, clientPos.Position.X, clientPos.Position.Y, clientPos.Position.Z)
		return
	}

	clientPos.Timestamp = float32(time.Now().UnixMilli()) / 1000.0
	m.broadcastMovement(conn, clientPos)
}

func isValidPosition(pos *gamepacket.Position) bool {
	return pos.X >= -1000 && pos.X <= 1000 &&
		pos.Y >= -10 && pos.Y <= 100 &&
		pos.Z >= -1000 && pos.Z <= 1000
}

func (m *MovementModule) broadcastMovement(conn net.PacketConn, clientPos *gamepacket.ClientPosition) {
	packet := &gamepacket.GamePacket{
		Seq:            uint32(rand.Intn(10000)),
		ClientPosition: clientPos,
	}

	// Broadcast to all clients EXCEPT the sender
	broadcastToAllExcept(conn, packet, clientPos.ClientId)
}

func broadcastToAllExcept(conn net.PacketConn, packet *gamepacket.GamePacket, excludeClientId string) {
	allClientsMu.RLock()
	defer allClientsMu.RUnlock()

	data, err := proto.Marshal(packet)
	if err != nil {
		fmt.Printf("❌ Failed to marshal movement packet: %v\n", err)
		return
	}

	sentCount := 0
	for _, client := range allClients {
		if client.PublicID == excludeClientId {
			continue
		}

		if _, err := conn.WriteTo(data, client.Address); err == nil {
			sentCount++
		}
	}

	// fmt.Printf("📡 Broadcasted movement to %d clients\n", sentCount)
}
