package main

import (
	"fmt"
	"gosocket/gamepacket"
	"net"
)

type ChatModule struct{}

func NewChatModule() *ChatModule {
	return &ChatModule{}
}

func (m *ChatModule) CanHandle(pkt *gamepacket.GamePacket) bool {
	return pkt.GetChatMessage() != nil
}

func (m *ChatModule) Handle(conn net.PacketConn, addr net.Addr, pkt *gamepacket.GamePacket) {
	chat := pkt.GetChatMessage()
	if chat == nil {
		return
	}

	senderExists := false
	allClientsMu.RLock()
	for _, client := range allClients {
		if client.PublicID == chat.ClientId {
			senderExists = true
			break
		}
	}
	allClientsMu.RUnlock()

	if !senderExists {
		fmt.Printf("⚠️ Chat from unknown client %s\n", chat.ClientId)
	}

	fmt.Printf("💬 %s says: %s\n", chat.ClientId, chat.Message)
	broadcastToAll(conn, pkt, "")
}
