package client

import (
	"fmt"
	"gosocket/internal/utils"
	"net"
	"strings"
	"sync"
	"time"
)

const (
	HeartbeatInterval = 10 * time.Second
	ClientTimeout     = 30 * time.Second
	PendingTimeout    = 30 * time.Second
)

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

func GetAllClients() []*ClientInfo {
	activeClientsMutex.RLock()
	defer activeClientsMutex.RUnlock()

	clients := make([]*ClientInfo, 0, len(activeClients))
	for _, client := range activeClients {
		clients = append(clients, client)
	}
	return clients
}

func GetClientCount() int {
	activeClientsMutex.RLock()
	defer activeClientsMutex.RUnlock()
	return len(activeClients)
}

func GetPendingClientCount() int {
	pendingClientsMutex.RLock()
	defer pendingClientsMutex.RUnlock()
	return len(pendingClients)
}

func AddClient(privateID, publicID, name string, addr net.Addr) {
	activeClientsMutex.Lock()
	defer activeClientsMutex.Unlock()

	sessionToken := utils.GenerateSessionToken()
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

// adds a pre-constructed client (for reconnections)
func AddClientDirect(client *ClientInfo) {
	activeClientsMutex.Lock()
	defer activeClientsMutex.Unlock()

	activeClients[client.PrivateID] = client
	fmt.Printf("✅ Client restored: %s (ID: %s)\n", client.PublicID, client.PrivateID)
}

func RemoveClient(conn net.PacketConn, privateID string) *ClientInfo {
	client := removeFromActiveClients(privateID)
	if client == nil {
		fmt.Printf("⚠️ Attempted to remove non-existent client: %s\n", privateID)
		return nil
	}

	cleanupClientData(client)

	if client.WasInLobby() {
		BroadcastPlayerLeft(conn, client)
	}

	fmt.Printf("❌ Client removed: %s (%s)\n", client.Name, client.PublicID)
	return client
}

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

func SetClientLobbyStatus(privateID string, inLobby bool) {
	activeClientsMutex.Lock()
	defer activeClientsMutex.Unlock()

	if client, exists := activeClients[privateID]; exists {
		client.InLobby = inLobby
		fmt.Printf("📍 Client %s lobby status: %t\n", client.PublicID, inLobby)
	}
}

// === Pending Client Management ===

// adds a client awaiting username submission
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

func GetLobbyPosition(publicID string) (LobbyPosition, bool) {
	lobbyPositionsMutex.RLock()
	defer lobbyPositionsMutex.RUnlock()

	pos, exists := lobbyPositions[publicID]
	return pos, exists
}

func SetLobbyPosition(publicID string, position LobbyPosition) {
	lobbyPositionsMutex.Lock()
	defer lobbyPositionsMutex.Unlock()

	lobbyPositions[publicID] = position
}

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

	lobbyPositionsMutex.Lock()
	delete(lobbyPositions, client.PublicID)
	lobbyPositionsMutex.Unlock()

	fmt.Printf("🗑️ Cleaned up lobby data for %s (wasn't in lobby)\n", client.PublicID)
}

func checkAndRemoveTimedOutClients(conn net.PacketConn) {
	now := time.Now()
	var timedOutClients []string

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

func GetAvailableColor() (string, bool) {
	activeClients := GetAllClients()
	usedColors := make([]string, 0, len(activeClients))
	for _, c := range activeClients {
		if c.ColorHex != "" {
			usedColors = append(usedColors, c.ColorHex)
		}
	}
	return utils.GetAvailableColor(usedColors)
}

func CleanupUnusedIDs() {
	activeClients := GetAllClients()

	activePrivateIDs := make(map[string]struct{})
	activePublicIDs := make(map[string]struct{})

	for _, c := range activeClients {
		activePrivateIDs[strings.ToLower(c.PrivateID)] = struct{}{}
		activePublicIDs[strings.ToLower(c.PublicID)] = struct{}{}
	}

	privateCleanedCount := utils.CleanupPrivateIDs(activePrivateIDs)
	publicCleanedCount := utils.CleanupPublicIDs(activePublicIDs)

	if privateCleanedCount > 0 || publicCleanedCount > 0 {
		fmt.Printf("🧹 Cleaned up %d private IDs, %d public IDs\n",
			privateCleanedCount, publicCleanedCount)
	}
}
