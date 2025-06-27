package main

import (
	"fmt"
	"gosocket/gamepacket"
	"math/rand"
	"net"
	"sync"
	"sync/atomic"

	"google.golang.org/protobuf/proto"
)

// BroadcastToAll sends a packet to all active clients except the excluded one
func BroadcastToAll(conn net.PacketConn, packet *gamepacket.GamePacket, excludePrivateID string) {
	data, err := proto.Marshal(packet)
	if err != nil {
		fmt.Printf("❌ Failed to marshal broadcast packet: %v\n", err)
		return
	}

	targets := getTargetClients(excludePrivateID)
	if len(targets) == 0 {
		return
	}

	sendConcurrently(conn, data, targets)
}

// BroadcastToAllExcept sends a packet to all clients except the one with given clientID
// This is used for movement packets where clientID is the public ID
func BroadcastToAllExcept(conn net.PacketConn, packet *gamepacket.GamePacket, excludeClientID string) {
	data, err := proto.Marshal(packet)
	if err != nil {
		fmt.Printf("❌ Failed to marshal movement packet: %v\n", err)
		return
	}

	targets := getTargetClientsByPublicID(excludeClientID)
	if len(targets) == 0 {
		return
	}

	sendConcurrently(conn, data, targets)
}

// SendToClient sends a packet to a specific client
func SendToClient(conn net.PacketConn, packet *gamepacket.GamePacket, client *ClientInfo) error {
	data, err := proto.Marshal(packet)
	if err != nil {
		return fmt.Errorf("failed to marshal packet: %w", err)
	}

	_, err = conn.WriteTo(data, client.Address)
	if err != nil {
		return fmt.Errorf("failed to send to %s: %w", client.PublicID, err)
	}

	return nil
}

// SendToAddress sends a packet to a specific network address
func SendToAddress(conn net.PacketConn, packet *gamepacket.GamePacket, addr net.Addr) error {
	data, err := proto.Marshal(packet)
	if err != nil {
		return fmt.Errorf("failed to marshal packet: %w", err)
	}

	_, err = conn.WriteTo(data, addr)
	if err != nil {
		return fmt.Errorf("failed to send to %s: %w", addr.String(), err)
	}

	return nil
}

// BroadcastToLobbyClients sends a packet only to clients currently in the lobby
func BroadcastToLobbyClients(conn net.PacketConn, packet *gamepacket.GamePacket, excludePrivateID string) {
	data, err := proto.Marshal(packet)
	if err != nil {
		fmt.Printf("❌ Failed to marshal lobby broadcast packet: %v\n", err)
		return
	}

	targets := getLobbyTargetClients(excludePrivateID)
	if len(targets) == 0 {
		return
	}

	sendConcurrently(conn, data, targets)
	fmt.Printf("📡 Lobby broadcast sent to %d clients\n", len(targets))
}

// === Helper Functions ===

// getTargetClients returns all clients except the excluded private ID
func getTargetClients(excludePrivateID string) []*ClientInfo {
	allClients := GetAllClients()

	targets := make([]*ClientInfo, 0, len(allClients))
	for _, client := range allClients {
		if client.PrivateID != excludePrivateID {
			targets = append(targets, client)
		}
	}

	return targets
}

// getTargetClientsByPublicID returns all clients except the excluded public ID
func getTargetClientsByPublicID(excludePublicID string) []*ClientInfo {
	allClients := GetAllClients()

	targets := make([]*ClientInfo, 0, len(allClients))
	for _, client := range allClients {
		if client.PublicID != excludePublicID {
			targets = append(targets, client)
		}
	}

	return targets
}

// getLobbyTargetClients returns all clients in lobby except the excluded private ID
func getLobbyTargetClients(excludePrivateID string) []*ClientInfo {
	allClients := GetAllClients()

	targets := make([]*ClientInfo, 0)
	for _, client := range allClients {
		if client.PrivateID != excludePrivateID && client.InLobby && client.ColorHex != "" {
			targets = append(targets, client)
		}
	}

	return targets
}

// sendConcurrently sends data to multiple clients using goroutines
func sendConcurrently(conn net.PacketConn, data []byte, targets []*ClientInfo) {
	var wg sync.WaitGroup
	sentCount := int32(0)

	for _, client := range targets {
		wg.Add(1)
		go func(c *ClientInfo) {
			defer wg.Done()

			if _, err := conn.WriteTo(data, c.Address); err == nil {
				atomic.AddInt32(&sentCount, 1)
			} else {
				fmt.Printf("⚠️ Failed to send to %s (%s): %v\n", c.PublicID, c.Address, err)
			}
		}(client)
	}

	wg.Wait()

	if len(targets) > 0 {
		fmt.Printf("📡 Broadcast sent to %d/%d clients\n", sentCount, len(targets))
	}
}

// === Utility Functions for Specific Broadcasts ===

// BroadcastServerMessage sends a server status message to all clients
func BroadcastServerMessage(conn net.PacketConn, message string) {
	packet := &gamepacket.GamePacket{
		Seq: generateRandomSeq(),
		ServerStatus: &gamepacket.ServerStatus{
			Message: message,
		},
	}

	BroadcastToAll(conn, packet, "")
	fmt.Printf("📢 Server message broadcast: %s\n", message)
}

// BroadcastPlayerJoined notifies all clients when a player joins the lobby
func BroadcastPlayerJoined(conn net.PacketConn, joinedClient *ClientInfo, lobbyJoinData *gamepacket.LobbyJoinBroadcast) {
	packet := &gamepacket.GamePacket{
		Seq: generateRandomSeq(),
		LobbyJoinBroadcast: &gamepacket.LobbyJoinBroadcast{
			PublicId:      lobbyJoinData.PublicId,
			Colorhex:      lobbyJoinData.Colorhex,
			Position:      lobbyJoinData.Position,
			IsLocalPlayer: false, // Always false for other players
		},
	}

	BroadcastToAll(conn, packet, joinedClient.PrivateID)
	fmt.Printf("📡 Player join broadcast: %s\n", joinedClient.PublicID)
}

// BroadcastMovement sends movement data to all clients except the moving client
func BroadcastMovement(conn net.PacketConn, clientPos *gamepacket.ClientPosition) {
	packet := &gamepacket.GamePacket{
		Seq:            generateRandomSeq(),
		ClientPosition: clientPos,
	}

	BroadcastToAllExcept(conn, packet, clientPos.ClientId)
}

// generateRandomSeq creates a random sequence number for packets
func generateRandomSeq() uint32 {
	return uint32(rand.Intn(100000))
}
