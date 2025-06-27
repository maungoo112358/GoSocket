package main

import (
	"fmt"
	"gosocket/gamepacket"
	"math/rand"
	"net"
	"sync"
	"time"
)

const (
	HeartbeatInterval = 10 * time.Second
	ClientTimeout     = 30 * time.Second
	PendingTimeout    = 30 * time.Second
)

// ClientInfo represents an active client connection
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

// PendingClient represents a client that hasn't completed username submission
type PendingClient struct {
	TempID    string
	Address   net.Addr
	CreatedAt time.Time
}

// LobbyPosition tracks where a client is positioned in the lobby
type LobbyPosition struct {
	X, Y, Z float64
}

// Client storage
var (
	activeClients      = make(map[string]*ClientInfo) // Key: privateID
	activeClientsMutex sync.RWMutex

	pendingClients      = make(map[string]*PendingClient) // Key: tempID
	pendingClientsMutex sync.RWMutex

	lobbyPositions      = make(map[string]LobbyPosition) // Key: publicID
	lobbyPositionsMutex sync.RWMutex
)

// === Public API ===

// GetAllClients returns a copy of all active clients
func GetAllClients() []*ClientInfo {
	activeClientsMutex.RLock()
	defer activeClientsMutex.RUnlock()

	clients := make([]*ClientInfo, 0, len(activeClients))
	for _, client := range activeClients {
		clients = append(clients, client)
	}
	return clients
}

// GetClientCount returns the number of active clients
func GetClientCount() int {
	activeClientsMutex.RLock()
	defer activeClientsMutex.RUnlock()
	return len(activeClients)
}

// GetPendingClientCount returns the number of pending clients
func GetPendingClientCount() int {
	pendingClientsMutex.RLock()
	defer pendingClientsMutex.RUnlock()
	return len(pendingClients)
}

// AddClient registers a new active client
func AddClient(privateID, publicID, name string, addr net.Addr) {
	activeClientsMutex.Lock()
	defer activeClientsMutex.Unlock()

	sessionToken := GenerateSessionToken()
	now := time.Now()

	activeClients[privateID] = &ClientInfo{
		PrivateID:     privateID,
		PublicID:      publicID,
		Name:          name,
		Address:       addr,
		LastHeartbeat: now,
		ConnectedAt:   now,
		SessionToken:  sessionToken,
		InLobby:       false,
	}

	fmt.Printf("✅ Client added: %s from %s (ID: %s)\n", publicID, addr, privateID)
}

// AddClientDirect adds a pre-constructed client (for reconnections)
func AddClientDirect(client *ClientInfo) {
	activeClientsMutex.Lock()
	defer activeClientsMutex.Unlock()

	activeClients[client.PrivateID] = client
	fmt.Printf("✅ Client restored: %s (ID: %s)\n", client.PublicID, client.PrivateID)
}

// RemoveClient disconnects and cleans up a client
func RemoveClient(conn net.PacketConn, privateID string) *ClientInfo {
	client := removeFromActiveClients(privateID)
	if client == nil {
		fmt.Printf("⚠️ Attempted to remove non-existent client: %s\n", privateID)
		return nil
	}

	cleanupClientData(client)

	if client.WasInLobby() {
		broadcastPlayerLeft(conn, client)
	}

	fmt.Printf("❌ Client removed: %s (%s)\n", client.Name, client.PublicID)
	return client
}

// UpdateHeartbeat updates the last heartbeat time for a client
func UpdateHeartbeat(privateID string) bool {
	activeClientsMutex.Lock()
	defer activeClientsMutex.Unlock()

	client, exists := activeClients[privateID]
	if !exists {
		return false
	}

	client.LastHeartbeat = time.Now()
	return true
}

// FindClientByPublicID searches for an active client by their public ID
func FindClientByPublicID(publicID string) *ClientInfo {
	activeClientsMutex.RLock()
	defer activeClientsMutex.RUnlock()

	for _, client := range activeClients {
		if client.PublicID == publicID {
			return client
		}
	}
	return nil
}

// SetClientLobbyStatus updates whether a client is in the lobby
func SetClientLobbyStatus(privateID string, inLobby bool) {
	activeClientsMutex.Lock()
	defer activeClientsMutex.Unlock()

	if client, exists := activeClients[privateID]; exists {
		client.InLobby = inLobby
		fmt.Printf("📍 Client %s lobby status: %t\n", client.PublicID, inLobby)
	}
}

// === Pending Client Management ===

// AddPendingClient adds a client awaiting username submission
func AddPendingClient(tempID string, addr net.Addr) {
	pendingClientsMutex.Lock()
	defer pendingClientsMutex.Unlock()

	pendingClients[tempID] = &PendingClient{
		TempID:    tempID,
		Address:   addr,
		CreatedAt: time.Now(),
	}

	fmt.Printf("📝 Pending client added: %s from %s\n", tempID, addr)
}

// RemovePendingClient removes a pending client by address
func RemovePendingClient(addr net.Addr) {
	pendingClientsMutex.Lock()
	defer pendingClientsMutex.Unlock()

	for tempID, client := range pendingClients {
		if client.Address.String() == addr.String() {
			delete(pendingClients, tempID)
			fmt.Printf("📝 Pending client removed: %s from %s\n", tempID, addr)
			return
		}
	}
}

// === Lobby Position Management ===

// GetLobbyPosition returns a client's lobby position
func GetLobbyPosition(publicID string) (LobbyPosition, bool) {
	lobbyPositionsMutex.RLock()
	defer lobbyPositionsMutex.RUnlock()

	pos, exists := lobbyPositions[publicID]
	return pos, exists
}

// SetLobbyPosition sets a client's lobby position
func SetLobbyPosition(publicID string, position LobbyPosition) {
	lobbyPositionsMutex.Lock()
	defer lobbyPositionsMutex.Unlock()

	lobbyPositions[publicID] = position
}

// GetAllLobbyPositions returns all lobby positions
func GetAllLobbyPositions() map[string]LobbyPosition {
	lobbyPositionsMutex.RLock()
	defer lobbyPositionsMutex.RUnlock()

	positions := make(map[string]LobbyPosition)
	for id, pos := range lobbyPositions {
		positions[id] = pos
	}
	return positions
}

// === Heartbeat System ===

// StartHeartbeatChecker begins monitoring client heartbeats
func StartHeartbeatChecker(conn net.PacketConn) {
	ticker := time.NewTicker(HeartbeatInterval)

	go func() {
		defer ticker.Stop()

		for range ticker.C {
			checkAndRemoveTimedOutClients(conn)
			cleanupExpiredPendingClients()
		}
	}()

	fmt.Println("💓 Heartbeat checker started")
}

// === Helper Methods ===

// WasInLobby checks if client was actively in lobby
func (c *ClientInfo) WasInLobby() bool {
	return c.InLobby && c.ColorHex != ""
}

// === Internal Functions ===

func removeFromActiveClients(privateID string) *ClientInfo {
	activeClientsMutex.Lock()
	defer activeClientsMutex.Unlock()

	client, exists := activeClients[privateID]
	if !exists {
		return nil
	}

	delete(activeClients, privateID)
	return client
}

func cleanupClientData(client *ClientInfo) {
	if client.InLobby {
		fmt.Printf("💾 Preserving lobby data for %s (was in lobby)\n", client.PublicID)
		return
	}

	// Remove lobby position for clients who weren't in lobby
	lobbyPositionsMutex.Lock()
	delete(lobbyPositions, client.PublicID)
	lobbyPositionsMutex.Unlock()

	fmt.Printf("🗑️ Cleaned up lobby data for %s (wasn't in lobby)\n", client.PublicID)
}

func checkAndRemoveTimedOutClients(conn net.PacketConn) {
	now := time.Now()
	var timedOutClients []string

	// Find timed out clients
	activeClientsMutex.RLock()
	for privateID, client := range activeClients {
		if now.Sub(client.LastHeartbeat) > ClientTimeout {
			timedOutClients = append(timedOutClients, privateID)
		}
	}
	activeClientsMutex.RUnlock()

	// Remove timed out clients
	if len(timedOutClients) > 0 {
		var wg sync.WaitGroup
		for _, privateID := range timedOutClients {
			wg.Add(1)
			go func(id string) {
				defer wg.Done()
				RemoveClient(conn, id)
			}(privateID)
		}
		wg.Wait()

		activeCount := GetClientCount()
		pendingCount := GetPendingClientCount()
		fmt.Printf("🔄 Removed %d timed out clients. Active: %d, Pending: %d\n",
			len(timedOutClients), activeCount, pendingCount)
	}
}

func cleanupExpiredPendingClients() {
	pendingClientsMutex.Lock()
	defer pendingClientsMutex.Unlock()

	now := time.Now()
	var expired []string

	for tempID, client := range pendingClients {
		if now.Sub(client.CreatedAt) > PendingTimeout {
			expired = append(expired, tempID)
		}
	}

	for _, tempID := range expired {
		client := pendingClients[tempID]
		delete(pendingClients, tempID)
		fmt.Printf("⏰ Expired pending client: %s from %s\n", tempID, client.Address)
	}

	if len(expired) > 0 {
		fmt.Printf("🗑️ Cleaned up %d expired pending clients\n", len(expired))
	}
}

func broadcastPlayerLeft(conn net.PacketConn, client *ClientInfo) {
	leavePacket := &gamepacket.GamePacket{
		Seq: uint32(rand.Intn(10000)),
		ServerStatus: &gamepacket.ServerStatus{
			Message:  fmt.Sprintf("Player %s left the lobby", client.PublicID),
			ClientId: client.PublicID,
		},
	}

	BroadcastToAll(conn, leavePacket, client.PrivateID)
	fmt.Printf("📤 Broadcast: %s left the lobby\n", client.PublicID)
}
