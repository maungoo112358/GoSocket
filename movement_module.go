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

// CanHandle determines if this module can process the packet
func (m *MovementModule) CanHandle(pkt *gamepacket.GamePacket) bool {
	return pkt.GetClientPosition() != nil
}

// Handle processes client movement packets
func (m *MovementModule) Handle(conn net.PacketConn, addr net.Addr, pkt *gamepacket.GamePacket) {
	clientPos := pkt.GetClientPosition()
	if clientPos == nil {
		return
	}

	// Validate the movement packet
	if !m.validateMovementPacket(clientPos, addr) {
		return
	}

	// Check rate limiting
	if !m.checkRateLimit(clientPos.ClientId) {
		fmt.Printf("⚠️ Rate limit exceeded for client %s\n", clientPos.ClientId)
		return
	}

	// Update timestamp and broadcast
	m.processMovement(conn, clientPos)
}

// === Movement Validation ===

func (m *MovementModule) validateMovementPacket(clientPos *gamepacket.ClientPosition, addr net.Addr) bool {
	// Check if client exists and is active
	if !m.isValidClient(clientPos.ClientId) {
		fmt.Printf("⚠️ Movement from unknown/inactive client: %s from %s\n",
			clientPos.ClientId, addr)
		return false
	}

	// Validate position bounds
	if !m.isValidPosition(clientPos.Position) {
		fmt.Printf("⚠️ Invalid position from %s: %.2f,%.2f,%.2f\n",
			clientPos.ClientId, clientPos.Position.X, clientPos.Position.Y, clientPos.Position.Z)
		return false
	}

	// Check for reasonable movement (optional anti-cheat)
	if !m.isReasonableMovement(clientPos) {
		fmt.Printf("⚠️ Suspicious movement from %s: teleport detected\n", clientPos.ClientId)
		// For now, just log - could implement stricter validation
	}

	return true
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

func (m *MovementModule) isReasonableMovement(clientPos *gamepacket.ClientPosition) bool {
	// TODO: Implement movement validation based on previous position and time
	// For now, always return true
	// Could check:
	// - Maximum speed between updates
	// - Physics constraints (can't move through walls)
	// - Teleport detection
	return true
}

// === Rate Limiting ===

func (m *MovementModule) checkRateLimit(clientID string) bool {
	now := time.Now()

	// Clean up old entries periodically
	m.cleanupRateLimitData(now)

	// Check if client is within rate limits
	lastSent, exists := m.rateLimiter.clientLastSent[clientID]
	if !exists {
		// First movement from this client
		m.rateLimiter.clientLastSent[clientID] = now
		m.rateLimiter.clientCounts[clientID] = 1
		return true
	}

	// Check if we're in a new time window
	if now.Sub(lastSent) >= MovementWindow {
		// Reset counter for new window
		m.rateLimiter.clientLastSent[clientID] = now
		m.rateLimiter.clientCounts[clientID] = 1
		return true
	}

	// Check if within rate limit for current window
	currentCount := m.rateLimiter.clientCounts[clientID]
	if float64(currentCount) >= MaxMovementRate {
		return false // Rate limit exceeded
	}

	// Update counter
	m.rateLimiter.clientCounts[clientID] = currentCount + 1
	return true
}

func (m *MovementModule) cleanupRateLimitData(now time.Time) {
	// Only cleanup every 10 seconds to avoid overhead
	// This is a simple approach - could be optimized with a proper cleanup schedule
	for clientID, lastSent := range m.rateLimiter.clientLastSent {
		if now.Sub(lastSent) > 10*time.Second {
			delete(m.rateLimiter.clientLastSent, clientID)
			delete(m.rateLimiter.clientCounts, clientID)
		}
	}
}

// === Movement Processing ===

func (m *MovementModule) processMovement(conn net.PacketConn, clientPos *gamepacket.ClientPosition) {
	// Update timestamp to server time
	clientPos.Timestamp = float32(time.Now().UnixMilli()) / 1000.0

	// Update client's lobby position if they're in lobby
	m.updateLobbyPosition(clientPos)

	// Broadcast to other clients
	m.broadcastMovement(conn, clientPos)
}

func (m *MovementModule) updateLobbyPosition(clientPos *gamepacket.ClientPosition) {
	// Only update lobby position if client is in lobby
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

	// Use the optimized broadcast function
	BroadcastToAllExcept(conn, packet, clientPos.ClientId)

	// Optional: Log movement for debugging (can be disabled in production)
	if m.shouldLogMovement(clientPos.ClientId) {
		fmt.Printf("📍 Movement: %s -> (%.1f, %.1f, %.1f)\n",
			clientPos.ClientId, clientPos.Position.X, clientPos.Position.Y, clientPos.Position.Z)
	}
}

func (m *MovementModule) shouldLogMovement(clientID string) bool {
	// Only log movement occasionally to avoid spam
	return false // Disabled by default
}

// === Statistics and Monitoring ===

func (m *MovementModule) GetStats() MovementStats {
	return MovementStats{
		ActiveClients:   len(m.rateLimiter.clientLastSent),
		TotalClients:    GetClientCount(),
		RateLimitWindow: MovementWindow,
		MaxRate:         MaxMovementRate,
	}
}

type MovementStats struct {
	ActiveClients   int
	TotalClients    int
	RateLimitWindow time.Duration
	MaxRate         float64
}

func (stats MovementStats) String() string {
	return fmt.Sprintf("Movement Stats: %d/%d clients active, max %.0f/sec",
		stats.ActiveClients, stats.TotalClients, stats.MaxRate)
}

// === Advanced Features (Future) ===

// MovementValidator interface for pluggable validation
type MovementValidator interface {
	ValidateMovement(prev, current *gamepacket.Position, deltaTime float64) bool
}

// PhysicsValidator implements basic physics-based movement validation
type PhysicsValidator struct {
	MaxSpeed float64 // units per second
}

func (pv *PhysicsValidator) ValidateMovement(prev, current *gamepacket.Position, deltaTime float64) bool {
	if prev == nil || current == nil || deltaTime <= 0 {
		return true // Can't validate
	}

	// Calculate distance moved
	dx := current.X - prev.X
	dy := current.Y - prev.Y
	dz := current.Z - prev.Z
	distance := math.Sqrt(dx*dx + dy*dy + dz*dz)

	// Calculate speed
	speed := distance / deltaTime

	return speed <= pv.MaxSpeed
}

// Future: Could add more validators
// - CollisionValidator: Check against world geometry
// - TeleportValidator: Detect impossible position changes
// - ZoneValidator: Ensure movement within allowed areas

// === Cleanup and Maintenance ===

func (m *MovementModule) Cleanup() {
	// Clean up rate limiter data
	now := time.Now()
	m.cleanupRateLimitData(now)

	fmt.Printf("🧹 Movement module cleanup completed\n")
}

// StartMaintenanceRoutine starts background cleanup
func (m *MovementModule) StartMaintenanceRoutine() {
	go func() {
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()

		for range ticker.C {
			m.Cleanup()
		}
	}()
}
