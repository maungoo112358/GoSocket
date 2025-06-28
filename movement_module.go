package main

import (
	"fmt"
	"gosocket/gamepacket"
	"math"
	"net"
	"time"
)

const (
	// Position validation bounds
	MinPositionX = -1000.0
	MaxPositionX = 1000.0
	MinPositionY = -10.0
	MaxPositionY = 100.0
	MinPositionZ = -1000.0
	MaxPositionZ = 1000.0

	// Rate limiting
	MaxMovementRate     = 60.0 // Max movements per second per client
	MovementWindow      = time.Second
	MaxPendingMovements = 100 // Max queued movements per client
)

const (
	// Collision detection settings
	PlayerRadius      = 0.5  // Each player occupies 0.1 unit radius
	CollisionGridSize = 10.0 // 10x10 unit grid for collision checking
)

type MovementModule struct {
	name        string
	rateLimiter *MovementRateLimiter
}

type MovementRateLimiter struct {
	clientLastSent map[string]time.Time
	clientCounts   map[string]int
}

func NewMovementModule() *MovementModule {
	return &MovementModule{
		name: "MovementModule",
		rateLimiter: &MovementRateLimiter{
			clientLastSent: make(map[string]time.Time),
			clientCounts:   make(map[string]int),
		},
	}
}

func (m *MovementModule) GetName() string {
	return m.name
}

func (m *MovementModule) CanHandle(pkt *gamepacket.GamePacket) bool {
	return pkt.GetClientPosition() != nil
}

func (m *MovementModule) Handle(conn net.PacketConn, addr net.Addr, pkt *gamepacket.GamePacket) {
	clientPos := pkt.GetClientPosition()
	if clientPos == nil {
		return
	}

	if !m.checkRateLimit(clientPos.ClientId) {
		fmt.Printf("⚠️ Rate limit exceeded for client %s\n", clientPos.ClientId)
		return
	}

	if !m.validateMovementPacket(clientPos, addr) {
		// Movement rejected - broadcast current position to ALL clients
		m.broadcastCurrentPosition(conn, clientPos.ClientId, pkt.Seq)
		return
	}

	m.processMovement(conn, clientPos)
}

// === Movement Validation ===
func (m *MovementModule) validateMovementPacket(clientPos *gamepacket.ClientPosition, addr net.Addr) bool {

	if !m.isValidClient(clientPos.ClientId) {
		fmt.Printf("⚠️ Movement from unknown/inactive client: %s from %s\n",
			clientPos.ClientId, addr)
		return false
	}

	if !m.isValidPosition(clientPos.Position) {
		fmt.Printf("⚠️ Invalid position from %s: %.2f,%.2f,%.2f\n",
			clientPos.ClientId, clientPos.Position.X, clientPos.Position.Y, clientPos.Position.Z)
		return false
	}

	if m.checkPlayerCollisions(clientPos.ClientId, clientPos.Position) {
		fmt.Printf("🚫 Movement rejected due to collision: %s\n", clientPos.ClientId)
		return false
	}

	return true
}

func (m *MovementModule) checkPlayerCollisions(clientID string, newPos *gamepacket.Position) bool {
	allPositions := GetAllLobbyPositions()

	logFlag := false
	playersInGrid := m.getPlayersInGrid(newPos, allPositions, clientID, &logFlag)

	for playerID, playerPos := range playersInGrid {
		if m.hasCollision(newPos, playerPos) {
			fmt.Printf("🚫 Collision detected: %s would collide with %s\n", clientID, playerID)
			return true
		}
	}

	return false
}

func (m *MovementModule) getPlayersInGrid(centerPos *gamepacket.Position, allPositions map[string]LobbyPosition, excludeClientID string, isLog *bool) map[string]LobbyPosition {
	gridHalfSize := CollisionGridSize / 2.0
	gridMinX := centerPos.X - gridHalfSize
	gridMaxX := centerPos.X + gridHalfSize
	gridMinZ := centerPos.Z - gridHalfSize
	gridMaxZ := centerPos.Z + gridHalfSize

	playersInGrid := make(map[string]LobbyPosition)

	for playerID, playerPos := range allPositions {
		// Skip the moving client
		if playerID == excludeClientID {
			continue
		}

		// Check if player is within grid bounds
		if m.isPositionInGrid(playerPos, gridMinX, gridMaxX, gridMinZ, gridMaxZ) {
			playersInGrid[playerID] = playerPos
		}
	}

	shouldLog := true
	if isLog != nil {
		shouldLog = *isLog
	}
	if shouldLog {
		fmt.Printf("🔍 Checking collision for %s: %d players in grid\n", excludeClientID, len(playersInGrid))
	}

	return playersInGrid
}

func (m *MovementModule) isPositionInGrid(pos LobbyPosition, minX, maxX, minZ, maxZ float64) bool {
	return pos.X >= minX && pos.X <= maxX && pos.Z >= minZ && pos.Z <= maxZ
}

func (m *MovementModule) hasCollision(pos1 *gamepacket.Position, pos2 LobbyPosition) bool {
	// Calculate distance on XZ plane (ignore Y for ground-based collision)
	dx := pos1.X - pos2.X
	dz := pos1.Z - pos2.Z
	distance := math.Sqrt(dx*dx + dz*dz)

	minDistance := PlayerRadius + PlayerRadius
	return distance < minDistance
}

func (m *MovementModule) broadcastCurrentPosition(conn net.PacketConn, clientID string, seq uint32) {
	currentPos, exists := GetLobbyPosition(clientID)
	if !exists {
		fmt.Printf("⚠️ Cannot broadcast position - no position found for %s\n", clientID)
		return
	}

	correctionPacket := &gamepacket.GamePacket{
		Seq: seq,
		ClientPosition: &gamepacket.ClientPosition{
			ClientId: clientID,
			Position: &gamepacket.Position{
				X: currentPos.X,
				Y: currentPos.Y,
				Z: currentPos.Z,
			},
			Timestamp: float32(time.Now().UnixMilli()) / 1000.0,
		},
	}

	BroadcastToAll(conn, correctionPacket, "")
	fmt.Printf("🚫 Collision rejected: broadcasted current position for %s to all clients\n", clientID)
}

func (m *MovementModule) isValidClient(clientID string) bool {
	client := FindClientByPublicID(clientID)
	return client != nil && client.InLobby
}

func (m *MovementModule) isValidPosition(pos *gamepacket.Position) bool {
	if pos == nil {
		return false
	}

	return pos.X >= MinPositionX && pos.X <= MaxPositionX &&
		pos.Y >= MinPositionY && pos.Y <= MaxPositionY &&
		pos.Z >= MinPositionZ && pos.Z <= MaxPositionZ
}

// === Rate Limiting ===

func (m *MovementModule) checkRateLimit(clientID string) bool {
	now := time.Now()

	m.cleanupRateLimitData(now)

	lastSent, exists := m.rateLimiter.clientLastSent[clientID]
	if !exists {
		m.rateLimiter.clientLastSent[clientID] = now
		m.rateLimiter.clientCounts[clientID] = 1
		return true
	}

	if now.Sub(lastSent) >= MovementWindow {
		m.rateLimiter.clientLastSent[clientID] = now
		m.rateLimiter.clientCounts[clientID] = 1
		return true
	}

	currentCount := m.rateLimiter.clientCounts[clientID]
	if float64(currentCount) >= MaxMovementRate {
		return false
	}

	m.rateLimiter.clientCounts[clientID] = currentCount + 1
	return true
}

func (m *MovementModule) cleanupRateLimitData(now time.Time) {
	for clientID, lastSent := range m.rateLimiter.clientLastSent {
		if now.Sub(lastSent) > 10*time.Second {
			delete(m.rateLimiter.clientLastSent, clientID)
			delete(m.rateLimiter.clientCounts, clientID)
		}
	}
}

// === Movement Processing ===

func (m *MovementModule) processMovement(conn net.PacketConn, clientPos *gamepacket.ClientPosition) {
	clientPos.Timestamp = float32(time.Now().UnixMilli()) / 1000.0

	m.updateLobbyPosition(clientPos)

	m.broadcastMovement(conn, clientPos)
}

func (m *MovementModule) updateLobbyPosition(clientPos *gamepacket.ClientPosition) {

	client := FindClientByPublicID(clientPos.ClientId)
	if client != nil && client.InLobby {
		newPos := LobbyPosition{
			X: clientPos.Position.X,
			Y: clientPos.Position.Y,
			Z: clientPos.Position.Z,
		}
		SetLobbyPosition(clientPos.ClientId, newPos)
	}
}

func (m *MovementModule) broadcastMovement(conn net.PacketConn, clientPos *gamepacket.ClientPosition) {
	packet := &gamepacket.GamePacket{
		Seq:            generateRandomSeq(),
		ClientPosition: clientPos,
	}

	logFlag := false
	BroadcastToAllExcept(conn, packet, clientPos.ClientId, &logFlag)
}
