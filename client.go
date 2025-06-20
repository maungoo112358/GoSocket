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
	ColorHex      string // For lobby state
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

	// Clean up from allClients
	delete(allClients, privateID)

	// Clean up lobby position
	allClientsLobbyPosMu.Lock()
	delete(allClientsLobbyPos, client.PublicID)
	allClientsLobbyPosMu.Unlock()

	allClientsMu.Unlock()

	// Clean up ID maps
	cleanupClientIDs(client.PrivateID, client.PublicID)

	fmt.Printf("❌ Client disconnected: %s (%s)\n", client.Name, client.PublicID)

	// Broadcast if they were in lobby
	if client.ColorHex != "" {
		broadcastPlayerLeft(conn, client) // ← Add conn parameter
	}

	return client
}

func getClient(privateID string) (*ClientInfo, bool) {
	allClientsMu.RLock()
	defer allClientsMu.RUnlock()

	client, exists := allClients[privateID]
	return client, exists
}

func getAllClients() map[string]*ClientInfo {
	allClientsMu.RLock()
	defer allClientsMu.RUnlock()

	// Return a copy to avoid holding the lock
	result := make(map[string]*ClientInfo)
	for k, v := range allClients {
		// Create a copy of the client info
		clientCopy := *v
		result[k] = &clientCopy
	}
	return result
}

func getClientCount() int {
	allClientsMu.RLock()
	defer allClientsMu.RUnlock()
	return len(allClients)
}

// Broadcast to all connected clients
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

// Send to specific client by privateID
func sendToClient(conn net.PacketConn, privateID string, packet *gamepacket.GamePacket) bool {
	client, exists := getClient(privateID)
	if !exists {
		return false
	}

	data, err := proto.Marshal(packet)
	if err != nil {
		return false
	}

	_, err = conn.WriteTo(data, client.Address)
	return err == nil
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
	ticker := time.NewTicker(10 * time.Second)
	go func() {
		for range ticker.C {
			now := time.Now()
			var toRemove []string

			allClientsMu.RLock()
			for privateID, client := range allClients {
				if now.Sub(client.LastHeartbeat) > 5*time.Second {
					toRemove = append(toRemove, privateID)
				}
			}
			allClientsMu.RUnlock()

			// Remove timed out clients
			for _, privateID := range toRemove {
				removeClient(conn, privateID) // ← Pass conn parameter
			}

			if len(toRemove) > 0 {
				fmt.Printf("🔄 Removed %d timed out clients. Total: %d\n", len(toRemove), getClientCount())
			}
		}
	}()
}

// Add these functions to your existing client.go file

// Get clients currently in lobby (have a color)
func getLobbyClients() map[string]*ClientInfo {
	allClientsMu.RLock()
	defer allClientsMu.RUnlock()

	result := make(map[string]*ClientInfo)
	for privateID, client := range allClients {
		if client.ColorHex != "" {
			clientCopy := *client
			result[privateID] = &clientCopy
		}
	}
	return result
}

// Get lobby statistics
func getLobbyStats() (int, int) {
	allClientsMu.RLock()
	defer allClientsMu.RUnlock()

	totalClients := len(allClients)
	lobbyClients := 0

	for _, client := range allClients {
		if client.ColorHex != "" {
			lobbyClients++
		}
	}

	return lobbyClients, totalClients
}

// Broadcast when a player leaves the lobby
func broadcastPlayerLeft(conn net.PacketConn, leftClient *ClientInfo) {
	if leftClient.ColorHex == "" {
		return // Client never joined lobby, no need to broadcast
	}

	leaveMessage := fmt.Sprintf("Player %s left the lobby", leftClient.PublicID)

	leavePacket := &gamepacket.GamePacket{
		Seq: uint32(rand.Intn(10000)),
		ServerStatus: &gamepacket.ServerStatus{
			Message:  leaveMessage,
			ClientId: leftClient.PublicID, // ← Add this field
		},
	}

	broadcastToAll(conn, leavePacket, leftClient.PrivateID)
	fmt.Printf("📤 Broadcast: %s left the lobby\n", leftClient.PublicID)
}
