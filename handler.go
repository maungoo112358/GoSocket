package main

import (
	"fmt"
	"gosocket/gamepacket"
	"net"

	"google.golang.org/protobuf/proto"
)

type PacketHandler func(conn net.PacketConn, addr net.Addr, pkt *gamepacket.GamePacket)

func dispatchPacket(conn net.PacketConn, addr net.Addr, pkt *gamepacket.GamePacket) {
	switch {
	case pkt.GetHandshakeRequest() != nil:
		handleHandshake(conn, addr, pkt)
	case pkt.GetHeartbeat() != nil:
		handleHeartbeat(conn, addr, pkt)
	case pkt.GetLobbyJoinBroadcast() != nil:
		handleLobbyJoin(conn, addr, pkt)
	case pkt.GetChatMessage() != nil:
		handleChatMessage(conn, addr, pkt)
	default:
		fmt.Println("⚠️ Unhandled packet type")
	}
}

func handleHandshake(conn net.PacketConn, addr net.Addr, pkt *gamepacket.GamePacket) {
	req := pkt.GetHandshakeRequest()
	if req == nil {
		return
	}

	privateID := generatePrivateID()
	publicID := generatePublicID()

	// Add to consolidated client map
	addClient(privateID, publicID, req.ClientName, addr)

	response := &gamepacket.GamePacket{
		Seq: pkt.Seq,
		HandshakeResponse: &gamepacket.HandshakeResponse{
			PrivateId: privateID,
			PublicId:  publicID,
		},
	}

	if data, err := proto.Marshal(response); err == nil {
		conn.WriteTo(data, addr)
	}
}

func handleHeartbeat(conn net.PacketConn, addr net.Addr, pkt *gamepacket.GamePacket) {
	heartbeat := pkt.GetHeartbeat()
	if heartbeat == nil {
		return
	}

	// Update heartbeat timestamp
	if !updateHeartbeat(heartbeat.ClientId) {
		fmt.Printf("⚠️ Invalid heartbeat from unknown client: %s\n", heartbeat.ClientId)
		return
	}

	// Send heartbeat acknowledgment
	response := &gamepacket.GamePacket{
		Seq: pkt.Seq,
		HeartbeatAck: &gamepacket.HeartbeatAck{
			ClientId: heartbeat.ClientId,
		},
	}

	if data, err := proto.Marshal(response); err == nil {
		conn.WriteTo(data, addr)
	}
}

func handleLobbyJoin(conn net.PacketConn, addr net.Addr, pkt *gamepacket.GamePacket) {
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

func handleChatMessage(conn net.PacketConn, addr net.Addr, pkt *gamepacket.GamePacket) {
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
}

func notifyClientsBeforeShutdown(conn net.PacketConn) {
	allClientsMu.Lock()
	defer allClientsMu.Unlock()

	for _, client := range allClients {
		response := &gamepacket.GamePacket{
			Seq: 999,
			ServerStatus: &gamepacket.ServerStatus{
				Message: "Server is shutting down",
			},
		}

		data, _ := proto.Marshal(response)
		conn.WriteTo(data, client.Address)
	}
}
