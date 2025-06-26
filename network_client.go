package main

import (
	crand "crypto/rand"
	"fmt"
	"gosocket/gamepacket"
	"math/rand"
	"net"
	"strings"
	"sync"
	"time"

	"google.golang.org/protobuf/proto"
)

var (
	privateIDStore = make(map[string]struct{})
	publicIDStore  = make(map[string]struct{})
	idMutex        sync.Mutex
)

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

func addClient(privateID, publicID, name string, addr net.Addr) {
	allClientsMu.Lock()
	defer allClientsMu.Unlock()

	sessionToken := generateSessionToken()

	allClients[privateID] = &ClientInfo{
		PrivateID:     privateID,
		PublicID:      publicID,
		Name:          name,
		Address:       addr,
		LastHeartbeat: time.Now(),
		ConnectedAt:   time.Now(),
		SessionToken:  sessionToken,
		InLobby:       false, // Initially not in lobby
	}

	fmt.Printf("✅ Client connected: %s from %s\n", publicID, addr)
}

func removeClient(conn net.PacketConn, privateID string) *ClientInfo {
	allClientsMu.Lock()
	client, exists := allClients[privateID]
	if !exists {
		allClientsMu.Unlock()
		return nil
	}

	delete(allClients, privateID)
	allClientsMu.Unlock()

	// Different behavior based on lobby status
	if client.InLobby {
		// Client was in lobby - wait 7 seconds for potential reconnection
		client.DisconnectedAt = time.Now()

		disconnectedClientsMu.Lock()
		disconnectedClients = append(disconnectedClients, client)
		disconnectedClientsMu.Unlock()

		fmt.Printf("🔄 Lobby client %s moved to disconnected list (reconnection window: %.0fs)\n",
			client.PublicID, reconnectionWindow.Seconds())

		// Wait for potential reconnection
		go func() {
			time.Sleep(reconnectionWindow)
			if !checkAndCleanupDisconnectedClient(client) {
				// Client didn't reconnect, broadcast leave and cleanup
				broadcastPlayerLeft(conn, client)
				cleanupClientData(client)
				fmt.Printf("❌ Lobby client %s permanently disconnected (session expired)\n", client.PublicID)
			}
		}()
	} else {
		// Client was not in lobby (only completed username) - immediate cleanup
		cleanupClientData(client)
		fmt.Printf("❌ Pre-lobby client disconnected: %s (%s) - immediate cleanup\n", client.Name, client.PublicID)
	}

	return client
}

func checkAndCleanupDisconnectedClient(client *ClientInfo) bool {
	disconnectedClientsMu.Lock()
	defer disconnectedClientsMu.Unlock()

	// Check if client is still in disconnected list (not reconnected)
	for i, dc := range disconnectedClients {
		if dc.PrivateID == client.PrivateID {
			// Remove from disconnected list
			disconnectedClients = append(disconnectedClients[:i], disconnectedClients[i+1:]...)
			return false // Client didn't reconnect
		}
	}

	return true // Client already reconnected
}

func cleanupClientData(client *ClientInfo) {
	// Clean up lobby position
	allClientsLobbyPosMu.Lock()
	delete(allClientsLobbyPos, client.PublicID)
	allClientsLobbyPosMu.Unlock()

	// Clean up ID mappings
	cleanupClientIDs(client.PrivateID, client.PublicID)
}

func generateSessionToken() string {
	return fmt.Sprintf("sess_%d_%d", time.Now().UnixNano(), rand.Intn(10000))
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

			// Check for timed out active clients
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

			// Clean up expired pending clients (those who never submitted username)
			cleanupExpiredPendingClients()

			// Clean up expired disconnected clients
			cleanupExpiredDisconnectedClients()

			if len(toRemove) > 0 {
				fmt.Printf("🔄 Removed %d timed out clients. Active: %d, Pending: %d, Disconnected: %d\n",
					len(toRemove), getClientCount(), getPendingClientCount(), getDisconnectedClientCount())
			}
		}
	}()
}

func cleanupExpiredDisconnectedClients() {
	disconnectedClientsMu.Lock()
	defer disconnectedClientsMu.Unlock()

	now := time.Now()
	var stillWaiting []*ClientInfo

	for _, client := range disconnectedClients {
		if now.Sub(client.DisconnectedAt) < reconnectionWindow {
			stillWaiting = append(stillWaiting, client)
		} else {
			// ✅ Ensure complete cleanup
			cleanupClientData(client)
			fmt.Printf("🗑️ Cleaned up expired session: %s\n", client.PublicID)
		}
	}
	disconnectedClients = stillWaiting
}

func broadcastPlayerLeft(conn net.PacketConn, leftClient *ClientInfo) {
	if leftClient.ColorHex == "" {
		return
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

func getDisconnectedClientCount() int {
	disconnectedClientsMu.RLock()
	defer disconnectedClientsMu.RUnlock()
	return len(disconnectedClients)
}

func generateSecureDigits(n int) string {
	b := make([]byte, n)
	crand.Read(b)
	for i := range b {
		b[i] = '0' + (b[i] % 10)
	}
	return string(b)
}

func generateUniqueID(prefix string, store map[string]struct{}) string {
	for {
		id := fmt.Sprintf("%s%s", prefix, generateSecureDigits(6))
		idlower := strings.ToLower(id)

		idMutex.Lock()
		_, exists := store[idlower]

		if !exists {
			store[idlower] = struct{}{}
			idMutex.Unlock()
			return id
		}
		idMutex.Unlock()
	}
}

func generatePrivateID() string {
	return generateUniqueID("Client_", privateIDStore)
}

func cleanupClientIDs(privateID, publicID string) {
	idMutex.Lock()
	defer idMutex.Unlock()

	delete(privateIDStore, strings.ToLower(privateID))
	delete(publicIDStore, strings.ToLower(publicID))
}
