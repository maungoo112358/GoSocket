package main

import (
	"fmt"
	"gosocket/gamepacket"
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
}

var (
	allClients   = make(map[string]*ClientInfo) // Key: privateID
	allClientsMu sync.RWMutex
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

func removeClient(privateID string) *ClientInfo {
	allClientsMu.Lock()
	defer allClientsMu.Unlock()

	client, exists := allClients[privateID]
	if exists {
		delete(allClients, privateID)
		fmt.Printf("❌ Client disconnected: %s (%s)\n", client.Name, client.PublicID)
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

func startHeartbeatChecker() {
	ticker := time.NewTicker(10 * time.Second)
	go func() {
		for range ticker.C {
			now := time.Now()
			var toRemove []string

			allClientsMu.RLock()
			for privateID, client := range allClients {
				if now.Sub(client.LastHeartbeat) > 30*time.Second {
					toRemove = append(toRemove, privateID)
				}
			}
			allClientsMu.RUnlock()

			// Remove timed out clients
			for _, privateID := range toRemove {
				removeClient(privateID)
			}

			if len(toRemove) > 0 {
				fmt.Printf("🔄 Removed %d timed out clients. Total: %d\n", len(toRemove), getClientCount())
			}
		}
	}()
}
