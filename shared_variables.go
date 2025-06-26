package main

import (
	"fmt"
	"math/rand"
	"net"
	"sync"
	"time"
)

type ClientInfo struct {
	PrivateID      string
	PublicID       string
	Name           string
	Address        net.Addr
	LastHeartbeat  time.Time
	ConnectedAt    time.Time
	ColorHex       string
	SessionToken   string
	DisconnectedAt time.Time
	InLobby        bool // Track if client has joined lobby
}

// Pending clients - waiting for username submission
type PendingClient struct {
	TempID    string
	Address   net.Addr
	CreatedAt time.Time
}

var (
	allClients   = make(map[string]*ClientInfo) // Key: privateID
	allClientsMu sync.RWMutex
)

// Pending clients system
var (
	pendingClients   = make(map[string]*PendingClient) // Key: tempID
	pendingClientsMu sync.RWMutex
	pendingTimeout   = 30 * time.Second // Time to submit username
)

// Reconnection system - updated timeouts
var (
	disconnectedClients   = make([]*ClientInfo, 0)
	disconnectedClientsMu sync.RWMutex
	reconnectionWindow    = 7 * time.Second // Reduced from 30 seconds
)

type LobbyPosition struct {
	X, Y, Z float64
}

var (
	allClientsLobbyPos   = make(map[string]LobbyPosition)
	allClientsLobbyPosMu sync.RWMutex
)

// Pending client management functions
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
