package main

import (
	"fmt"
	"gosocket/gamepacket"
	"net"
)

type LobbyModule struct{}

func NewLobbyModule() *LobbyModule {
	return &LobbyModule{}
}

func (m *LobbyModule) CanHandle(pkt *gamepacket.GamePacket) bool {
	return pkt.GetLobbyJoinBroadcast() != nil
}

func (m *LobbyModule) Handle(conn net.PacketConn, addr net.Addr, pkt *gamepacket.GamePacket) {
	lobby := pkt.GetLobbyJoinBroadcast()
	if lobby == nil {
		return
	}

	allClientsMu.Lock()
	var senderPrivateID string
	for privateID, client := range allClients {
		if client.PublicID == lobby.PublicId {
			client.ColorHex = lobby.ColorHex
			senderPrivateID = privateID
			break
		}
	}
	allClientsMu.Unlock()

	if senderPrivateID == "" {
		fmt.Printf("⚠️ Lobby join from unknown client:  %s\n", lobby.PublicId)
		return
	}

	fmt.Printf("🎨 %s joined lobby with color %s\n", lobby.PublicId, lobby.ColorHex)
	broadcastToAll(conn, pkt, senderPrivateID)
}
