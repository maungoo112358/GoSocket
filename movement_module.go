package main

import (
	"fmt"
	"gosocket/gamepacket"
	"net"
	"sync"
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
	MaxMovementRate     = 60.0
	MovementWindow      = time.Second
	MaxPendingMovements = 100

	// Concurrency settings
	MaxMovementWorkers = 10  // Number of concurrent movement processors
	MovementBufferSize = 100 // Buffer size for movement queue
)

type MovementModule struct {
	name        string
	rateLimiter *MovementRateLimiter

	collisionService CollisionService

	// Concurrency components
	movementQueue chan MovementRequest
	workerPool    sync.WaitGroup
	isRunning     bool
	stopChan      chan struct{}
	mu            sync.RWMutex // Protects shared state
}

type MovementRequest struct {
	conn      net.PacketConn
	addr      net.Addr
	packet    *gamepacket.GamePacket
	clientPos *gamepacket.ClientPosition
}

type MovementRateLimiter struct {
	clientLastSent map[string]time.Time
	clientCounts   map[string]int
	mu             sync.RWMutex // Protects rate limiter maps
}

func NewMovementModule() *MovementModule {
	RegisterModule(ModuleInfo{
		Name:         MovementModuleEnum,
		Type:         NonCritical,
		Dependencies: []ModuleEnum{},
		SubModules:   []ModuleEnum{},
	})

	var collisionService CollisionService
	if service := GetService(CollisionModuleEnum); service != nil {
		collisionService = service.(CollisionService)
	}

	return &MovementModule{
		name:             "MovementModule",
		collisionService: collisionService,
		rateLimiter: &MovementRateLimiter{
			clientLastSent: make(map[string]time.Time),
			clientCounts:   make(map[string]int),
		},
		movementQueue: make(chan MovementRequest, MovementBufferSize),
		stopChan:      make(chan struct{}),
		isRunning:     false,
	}
}

func (m *MovementModule) StartWorkers() {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.isRunning {
		return
	}

	m.isRunning = true

	// Start worker goroutines
	for i := 0; i < MaxMovementWorkers; i++ {
		m.workerPool.Add(1)
		go m.movementWorker(i)
	}

	fmt.Printf("🚀 Started %d movement workers\n", MaxMovementWorkers)
}

func (m *MovementModule) StopWorkers() {
	m.mu.Lock()
	defer m.mu.Unlock()

	if !m.isRunning {
		return
	}

	m.isRunning = false
	close(m.stopChan)

	// Wait for all workers to finish
	m.workerPool.Wait()

	fmt.Println("🛑 All movement workers stopped")
}

func (m *MovementModule) movementWorker(workerID int) {
	defer m.workerPool.Done()

	fmt.Printf("🔧 Movement worker %d started\n", workerID)

	for {
		select {
		case req := <-m.movementQueue:
			m.processMovementRequest(req, workerID)
		case <-m.stopChan:
			fmt.Printf("🔧 Movement worker %d stopping\n", workerID)
			return
		}
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

	req := MovementRequest{
		conn:      conn,
		addr:      addr,
		packet:    pkt,
		clientPos: clientPos,
	}

	// Queue for concurrent processing
	select {
	case m.movementQueue <- req:
	default:
		fmt.Printf("⚠️ Movement queue full, processing synchronously for %s\n", clientPos.ClientId)
		m.processMovementRequest(req, -1) // -1 indicates synchronous processing
	}
}

func (m *MovementModule) processMovementRequest(req MovementRequest, workerID int) {
	clientPos := req.clientPos

	if !m.checkRateLimit(clientPos.ClientId) {
		fmt.Printf("⚠️ [Worker %d] Rate limit exceeded for client %s\n", workerID, clientPos.ClientId)
		return
	}

	if !m.validateMovementPacket(clientPos, req.addr, req) {
		return
	}

	m.processMovement(req.conn, clientPos)
}

// === Movement Validation ===
func (m *MovementModule) validateMovementPacket(clientPos *gamepacket.ClientPosition, addr net.Addr, req MovementRequest) bool {
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

	if m.collisionService != nil && m.collisionService.CheckPlayerCollision(clientPos.ClientId, clientPos.Position) {
		m.collisionService.BroadcastRejection(req.conn, clientPos.ClientId, req.packet.Seq)
		return false
	}

	return true
}

func (m *MovementModule) isValidClient(clientID string) bool {
	client := FindClientByPublicID(clientID)
	return client != nil && client.InLobby
}

func (m *MovementModule) isValidPosition(pos *gamepacket.Vector_3) bool {
	if pos == nil {
		return false
	}

	return pos.X >= MinPositionX && pos.X <= MaxPositionX &&
		pos.Y >= MinPositionY && pos.Y <= MaxPositionY &&
		pos.Z >= MinPositionZ && pos.Z <= MaxPositionZ
}

// === Rate Limiting ===
func (m *MovementModule) checkRateLimit(clientID string) bool {
	m.rateLimiter.mu.Lock()
	defer m.rateLimiter.mu.Unlock()

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

// === Statistics ===
func (m *MovementModule) GetStats() MovementStats {
	m.rateLimiter.mu.RLock()
	activeClients := len(m.rateLimiter.clientLastSent)
	m.rateLimiter.mu.RUnlock()

	return MovementStats{
		ActiveClients:   activeClients,
		TotalClients:    GetClientCount(),
		RateLimitWindow: MovementWindow,
		MaxRate:         MaxMovementRate,
		QueueSize:       len(m.movementQueue),
		WorkerCount:     MaxMovementWorkers,
	}
}

type MovementStats struct {
	ActiveClients   int
	TotalClients    int
	RateLimitWindow time.Duration
	MaxRate         float64
	QueueSize       int
	WorkerCount     int
}

func (stats MovementStats) String() string {
	return fmt.Sprintf("Movement Stats: %d/%d clients active, max %.0f/sec, queue: %d, workers: %d",
		stats.ActiveClients, stats.TotalClients, stats.MaxRate, stats.QueueSize, stats.WorkerCount)
}
