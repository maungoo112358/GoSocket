package main

import (
	crand "crypto/rand"
	"fmt"
	"gosocket/gamepacket"
	"math/rand"
	"net"
	"strings"
	"sync"
	"sync/atomic"
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
	SessionToken  string
	InLobby       bool
}

var (
	allClients   = make(map[string]*ClientInfo) // Key: privateID
	allClientsMu sync.RWMutex
)

type PendingClient struct {
	TempID    string
	Address   net.Addr
	CreatedAt time.Time
}

var (
	pendingClients   = make(map[string]*PendingClient) // Key: tempID
	pendingClientsMu sync.RWMutex
	pendingTimeout   = 30 * time.Second // Time to submit username
)

type LobbyPosition struct {
	X, Y, Z float64
}

var (
	allClientsLobbyPos   = make(map[string]LobbyPosition)
	allClientsLobbyPosMu sync.RWMutex
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

	// Collect target clients
	var targets []*ClientInfo
	for privateID, client := range allClients {
		if privateID != excludePrivateID {
			targets = append(targets, client)
		}
	}
	allClientsMu.RUnlock()

	if len(targets) == 0 {
		return
	}

	var wg sync.WaitGroup
	sentCount := int32(0)

	// Send to each client concurrently
	for _, client := range targets {
		wg.Add(1)
		go func(c *ClientInfo) {
			defer wg.Done()
			if _, err := conn.WriteTo(data, c.Address); err == nil {
				atomic.AddInt32(&sentCount, 1)
			}
		}(client)
	}

	wg.Wait()
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
		InLobby:       false,
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

	// Clean up client data immediately - no more disconnected clients system
	cleanupClientData(client)

	// Broadcast player left if they were in lobby
	if client.InLobby {
		broadcastPlayerLeft(conn, client)
	}

	fmt.Printf("❌ Client disconnected: %s (%s) - immediate cleanup\n", client.Name, client.PublicID)
	return client
}

func cleanupClientData(client *ClientInfo) {
	// Clean up lobby position ONLY if client wasn't in lobby
	// This preserves position for reconnection if they were in lobby
	if !client.InLobby {
		allClientsLobbyPosMu.Lock()
		delete(allClientsLobbyPos, client.PublicID)
		allClientsLobbyPosMu.Unlock()
		fmt.Printf("🗑️ Cleaned up position data for %s (wasn't in lobby)\n", client.PublicID)
	} else {
		fmt.Printf("💾 Preserving position data for %s (was in lobby)\n", client.PublicID)
	}

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
				if now.Sub(client.LastHeartbeat) > 15*time.Second {
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

			if len(toRemove) > 0 {
				fmt.Printf("🔄 Removed %d timed out clients. Active: %d, Pending: %d\n",
					len(toRemove), getClientCount(), getPendingClientCount())
			}
		}
	}()
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

func addPendingClient(tempID string, addr net.Addr) {
	pendingClientsMu.Lock()
	defer pendingClientsMu.Unlock()

	pendingClients[tempID] = &PendingClient{
		TempID:    tempID,
		Address:   addr,
		CreatedAt: time.Now(),
	}

	fmt.Printf("📝 Added pending client %s from %s\n", tempID, addr)
}

func removePendingClient(addr net.Addr) {
	pendingClientsMu.Lock()
	defer pendingClientsMu.Unlock()

	// Find and remove pending client by address
	for tempID, client := range pendingClients {
		if client.Address.String() == addr.String() {
			delete(pendingClients, tempID)
			fmt.Printf("📝 Removed pending client %s from %s\n", tempID, addr)
			return
		}
	}
}

func generateTempID() string {
	return fmt.Sprintf("temp_%d_%d", time.Now().UnixNano(), rand.Intn(10000))
}

// Cleanup expired pending clients
func cleanupExpiredPendingClients() {
	pendingClientsMu.Lock()
	defer pendingClientsMu.Unlock()

	now := time.Now()
	var toRemove []string

	for tempID, client := range pendingClients {
		if now.Sub(client.CreatedAt) > pendingTimeout {
			toRemove = append(toRemove, tempID)
		}
	}

	for _, tempID := range toRemove {
		client := pendingClients[tempID]
		delete(pendingClients, tempID)
		fmt.Printf("⏰ Removed expired pending client %s from %s\n", tempID, client.Address)
	}

	if len(toRemove) > 0 {
		fmt.Printf("🗑️ Cleaned up %d expired pending clients\n", len(toRemove))
	}
}

func getPendingClientCount() int {
	pendingClientsMu.RLock()
	defer pendingClientsMu.RUnlock()
	return len(pendingClients)
}
