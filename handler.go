package main

import (
	"fmt"
	"gosocket/gamepacket"
	"net"
	"sync"
	"time"

	"google.golang.org/protobuf/proto"
)

type PacketHandler func(conn net.PacketConn, addr net.Addr, pkt *gamepacket.GamePacket)

var (
	clientMap   = make(map[string]string)
	clientMutex sync.Mutex
)

func dispatchPacket(conn net.PacketConn, addr net.Addr, pkt *gamepacket.GamePacket) {
	switch {
	case pkt.GetHandshakeRequest() != nil:
		handleHandshake(conn, addr, pkt)
	case pkt.GetHeartbeat() != nil:
		handleHeartbeat(conn, addr, pkt)
	case pkt.GetLobbyJoinBroadcast() != nil:
		handleLobbyJoin(conn, addr, pkt)
	case pkt.GetChatMessage() != nil:
		handleChat(conn, addr, pkt)
	default:
		fmt.Println("⚠️ Unhandled packet type")
	}
}

func handleHandshake(conn net.PacketConn, addr net.Addr, pkt *gamepacket.GamePacket) {
	handshake := pkt.GetHandshakeRequest()
	if handshake == nil {
		fmt.Println("⚠️ Invalid handshake packet")
		return
	}
	privateID := generatePrivateID()
	publicID := generatePublicID()

	response := &gamepacket.GamePacket{
		Seq: pkt.Seq,
		HandshakeResponse: &gamepacket.HandshakeResponse{
			PrivateId: privateID,
			PublicId:  publicID,
		},
	}

	data, err := proto.Marshal(response)
	if err == nil {
		conn.WriteTo(data, addr)
	}
	clientsMu.Lock()
	clients[privateID] = &Client{
		PrivateID: privateID,
		PublicID:  publicID,
		Addr:      addr,
		LastSeen:  time.Now(),
	}
	clientsMu.Unlock()

	clientMutex.Lock()
	clientMap[privateID] = publicID
	clientMutex.Unlock()
	fmt.Printf("🟢 New Client connected from %s\n Private ID %s\n Public ID %s\n", addr.String(), privateID, publicID)
}

func handleHeartbeat(conn net.PacketConn, addr net.Addr, pkt *gamepacket.GamePacket) {
	heartbeat := pkt.GetHeartbeat()
	if heartbeat == nil {
		return
	}
	updateHeartbeat(heartbeat.ClientId)
}

func handleLobbyJoin(conn net.PacketConn, addr net.Addr, pkt *gamepacket.GamePacket) {
	join := pkt.GetLobbyJoinBroadcast()
	if join == nil {
		return
	}
	fmt.Printf("🏠 %s joined the lobby with color  %s\n", join.PublicId, join.ColorHex)
}

func handleDisconnection(privateID string) {
	clientsMu.Lock()
	delete(clients, privateID)
	clientsMu.Unlock()

	clientMutex.Lock()
	publicID, exists := clientMap[privateID]
	if exists {
		fmt.Printf("🔴 Client disconnected %s (%s)\n", publicID, privateID)
		delete(clientMap, privateID)
	}
	clientMutex.Unlock()
}

func handleChat(conn net.PacketConn, addr net.Addr, pkt *gamepacket.GamePacket) {
	msg := pkt.GetChatMessage()
	if msg != nil {
		fmt.Printf("💬 %s says: %s\n", msg.ClientId, msg.Message)
	}
}

func notifyClientsBeforeShutdown(conn net.PacketConn) {
	clientsMu.Lock()
	defer clientsMu.Unlock()

	for _, client := range clients {
		response := &gamepacket.GamePacket{
			Seq: 999,
			ServerStatus: &gamepacket.ServerStatus{
				Message: "Server is shutting down",
			},
		}

		data, _ := proto.Marshal(response)
		conn.WriteTo(data, client.Addr)
	}
}
