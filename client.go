package main

import (
	"fmt"
	"gosocket/gamepacket"
	"math/rand"
	"net"
	"sync"
	"time"

	"google.golang.org/protobuf/proto"
)

type ClientInfo struct {
	PrivateID     string
	PublicID      string
	Name          string
	Address       net.Addr
	LastHeartbeat time.Time
	ConnectedAt   time.Time
	ColorHex      string
	ColorHex_Head string
}

var (
	allClients   = make(map[string]*ClientInfo) // Key: privateID
	allClientsMu sync.RWMutex
)

type LobbyPosition struct {
	X, Y, Z float64
}

var (
	allClientsLobbyPos   = make(map[string]LobbyPosition)
	allClientsLobbyPosMu sync.RWMutex
)

func addClient(privateID, publicID, name string, addr net.Addr) {
	allClientsMu.Lock()
	defer allClientsMu.Unlock()

	allClients[privateID] = &ClientInfo{
		PrivateID:     privateID,
		PublicID:      publicID,
		Name:          name,
		Address:       addr,
		LastHeartbeat: time.Now(),
		ConnectedAt:   time.Now(),
	}

	fmt.Printf("✅ Client connected: %s (%s) from %s\n", name, publicID, addr)
}

func removeClient(conn net.PacketConn, privateID string) *ClientInfo {
	allClientsMu.Lock()
	client, exists := allClients[privateID]
	if !exists {
		allClientsMu.Unlock()
		return nil
	}

	delete(allClients, privateID)

	allClientsLobbyPosMu.Lock()
	delete(allClientsLobbyPos, client.PublicID)
	allClientsLobbyPosMu.Unlock()

	allClientsMu.Unlock()

	cleanupClientIDs(client.PrivateID, client.PublicID)

	fmt.Printf("❌ Client disconnected: %s (%s)\n", client.Name, client.PublicID)

	//when client is in the lobby
	if client.ColorHex != "" {
		broadcastPlayerLeft(conn, client)
	}

	return client
}

func broadcastToAll(conn net.PacketConn, packet *gamepacket.GamePacket, excludePrivateID string) {
	data, err := proto.Marshal(packet)
	if err != nil {
		fmt.Printf("Failed to marshal broadcast packet: %v\n", err)
		return
	}

	allClientsMu.RLock()
	defer allClientsMu.RUnlock()

	sentCount := 0
	for privateID, client := range allClients {
		if privateID != excludePrivateID {
			if _, err := conn.WriteTo(data, client.Address); err == nil {
				sentCount++
			}
		}
	}

	fmt.Printf("📡 Broadcast sent to %d clients\n", sentCount)
}

func updateHeartbeat(privateID string) bool {
	allClientsMu.Lock()
	defer allClientsMu.Unlock()

	client, exists := allClients[privateID]
	if exists {
		client.LastHeartbeat = time.Now()
	}
	return exists
}

func startHeartbeatChecker(conn net.PacketConn) {
	ticker := time.NewTicker(5 * time.Second)
	go func() {
		for range ticker.C {
			now := time.Now()
			var toRemove []string

			allClientsMu.RLock()
			for privateID, client := range allClients {
				if now.Sub(client.LastHeartbeat) > 10*time.Second {
					toRemove = append(toRemove, privateID)
				}
			}
			allClientsMu.RUnlock()

			// Remove timed out clients
			for _, privateID := range toRemove {
				removeClient(conn, privateID)
			}

			if len(toRemove) > 0 {
				fmt.Printf("🔄 Removed %d timed out clients. Total: %d\n", len(toRemove), getClientCount())
			}
		}
	}()
}

func broadcastPlayerLeft(conn net.PacketConn, leftClient *ClientInfo) {
	if leftClient.ColorHex == "" {
		return // Client never joined lobby, no need to broadcast
	}

	leaveMessage := fmt.Sprintf("Player %s left the lobby", leftClient.PublicID)

	leavePacket := &gamepacket.GamePacket{
		Seq: uint32(rand.Intn(10000)),
		ServerStatus: &gamepacket.ServerStatus{
			Message:  leaveMessage,
			ClientId: leftClient.PublicID,
		},
	}

	broadcastToAll(conn, leavePacket, leftClient.PrivateID)
	fmt.Printf("📤 Broadcast: %s left the lobby\n", leftClient.PublicID)
}

func getClientCount() int {
	allClientsMu.RLock()
	defer allClientsMu.RUnlock()
	return len(allClients)
}
