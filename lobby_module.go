package main

import (
	"fmt"
	"gosocket/gamepacket"
	"math"
	"math/rand"
	"net"
)

const (
	DefaultLobbyY       = 1.0
	MinPositionDistance = 5.0
	MaxPositionRetries  = 100
	LobbyBoundary       = 5.0 // Area from -5 to +5 on X and Z axes
)

type LobbyModule struct{}

func NewLobbyModule() *LobbyModule {
	RegisterModule(ModuleInfo{
		Name:         LobbyModuleEnum,
		Type:         Critical,
		Dependencies: []ModuleEnum{},
		SubModules:   []ModuleEnum{},
	})
	return &LobbyModule{}
}

// CanHandle determines if this module can process the packet
func (m *LobbyModule) CanHandle(pkt *gamepacket.GamePacket) bool {
	return pkt.GetLobbyJoinBroadcast() != nil
}

// Handle processes lobby join requests
func (m *LobbyModule) Handle(conn net.PacketConn, addr net.Addr, pkt *gamepacket.GamePacket) {
	lobbyJoin := pkt.GetLobbyJoinBroadcast()
	if lobbyJoin == nil {
		return
	}

	fmt.Printf("🔄 LOBBY JOIN: %s with color %s\n", lobbyJoin.PublicId, lobbyJoin.Colorhex)

	// Find and prepare the joining client
	joiningClient := m.prepareJoiningClient(lobbyJoin.PublicId)
	if joiningClient == nil {
		fmt.Printf("⚠️ Lobby join from unknown client: %s\n", lobbyJoin.PublicId)
		return
	}

	// Handle client position (new or existing)
	position := m.getOrCreatePosition(joiningClient)

	// Update lobby join data with client info
	m.updateLobbyJoinData(lobbyJoin, joiningClient, position)

	// Send responses to clients
	m.handleLobbyJoinNotifications(conn, joiningClient, lobbyJoin)
}

// === Client Preparation ===

func (m *LobbyModule) prepareJoiningClient(publicID string) *ClientInfo {
	client := FindClientByPublicID(publicID)
	if client == nil {
		return nil
	}

	fmt.Printf("🔍 Found client %s, current color: '%s', in lobby: %t\n",
		publicID, client.ColorHex, client.InLobby)

	// Mark client as being in lobby
	SetClientLobbyStatus(client.PrivateID, true)

	// Assign color if needed
	m.ensureClientHasColor(client)

	return client
}

func (m *LobbyModule) ensureClientHasColor(client *ClientInfo) {
	if client.ColorHex != "" {
		return // Already has color
	}

	color, available := getAvailableColor()
	if !available {
		fmt.Printf("⚠️ No available colors for %s, using random color\n", client.PublicID)
		color = getRandomColor()
	}

	client.ColorHex = color
	fmt.Printf("🎨 Assigned color %s to %s\n", color, client.PublicID)
}

// === Position Management ===

func (m *LobbyModule) getOrCreatePosition(client *ClientInfo) LobbyPosition {
	// Check for existing position (reconnection case)
	if existingPos, hasPosition := GetLobbyPosition(client.PublicID); hasPosition {
		fmt.Printf("🔄 Restoring %s to previous position (%.2f, %.2f, %.2f)\n",
			client.PublicID, existingPos.X, existingPos.Y, existingPos.Z)
		return existingPos
	}

	// Generate new position for first-time join
	newPos := m.generateUniquePosition(client.PublicID)
	fmt.Printf("🎨 %s joined lobby with new position (%.2f, %.2f, %.2f)\n",
		client.PublicID, newPos.X, newPos.Y, newPos.Z)

	return newPos
}

func (m *LobbyModule) generateUniquePosition(publicID string) LobbyPosition {
	minDistance := MinPositionDistance

	// Try with decreasing minimum distance requirements
	for minDistance >= 0.5 {
		for attempt := 0; attempt < MaxPositionRetries; attempt++ {
			position := m.generateRandomPosition()

			if !m.hasPositionCollision(position, minDistance) {
				SetLobbyPosition(publicID, position)
				return position
			}
		}
		// Reduce distance requirement and try again
		minDistance -= 0.5
	}

	// Fallback to origin if all attempts failed
	fallbackPos := LobbyPosition{X: 0, Y: DefaultLobbyY, Z: 0}
	SetLobbyPosition(publicID, fallbackPos)
	fmt.Printf("⚠️ Using fallback position for %s\n", publicID)
	return fallbackPos
}

func (m *LobbyModule) generateRandomPosition() LobbyPosition {
	return LobbyPosition{
		X: (rand.Float64()*2 - 1) * LobbyBoundary, // Random between -5 and +5
		Y: DefaultLobbyY,
		Z: (rand.Float64()*2 - 1) * LobbyBoundary, // Random between -5 and +5
	}
}

func (m *LobbyModule) hasPositionCollision(newPos LobbyPosition, minDistance float64) bool {
	allPositions := GetAllLobbyPositions()

	for _, existingPos := range allPositions {
		distance := m.calculateDistance(newPos, existingPos)
		if distance < minDistance {
			return true
		}
	}

	return false
}

func (m *LobbyModule) calculateDistance(pos1, pos2 LobbyPosition) float64 {
	dx := pos1.X - pos2.X
	dy := pos1.Y - pos2.Y
	dz := pos1.Z - pos2.Z
	return math.Sqrt(dx*dx + dy*dy + dz*dz)
}

// === Lobby Join Data Management ===

func (m *LobbyModule) updateLobbyJoinData(lobbyJoin *gamepacket.LobbyJoinBroadcast, client *ClientInfo, position LobbyPosition) {
	// Update with client's assigned color
	lobbyJoin.Colorhex = client.ColorHex

	// Set position data
	lobbyJoin.Position = &gamepacket.ClientLobbyPosition{
		Position: &gamepacket.Position{
			X: position.X,
			Y: position.Y,
			Z: position.Z,
		},
	}
}

// === Notification System ===

func (m *LobbyModule) handleLobbyJoinNotifications(conn net.PacketConn, joiningClient *ClientInfo, lobbyJoin *gamepacket.LobbyJoinBroadcast) {
	// Send confirmation to the joining client
	m.sendLocalPlayerConfirmation(conn, joiningClient, lobbyJoin)

	// Notify other clients about the new player
	m.broadcastPlayerJoin(conn, joiningClient, lobbyJoin)

	// Send welcome messages and lobby state
	m.sendWelcomeSequence(conn, joiningClient)
}

func (m *LobbyModule) sendLocalPlayerConfirmation(conn net.PacketConn, client *ClientInfo, lobbyJoin *gamepacket.LobbyJoinBroadcast) {
	localPacket := &gamepacket.GamePacket{
		Seq: generateRandomSeq(),
		LobbyJoinBroadcast: &gamepacket.LobbyJoinBroadcast{
			PublicId:      lobbyJoin.PublicId,
			Colorhex:      lobbyJoin.Colorhex,
			Position:      lobbyJoin.Position,
			IsLocalPlayer: true,
		},
	}

	if err := SendToClient(conn, localPacket, client); err != nil {
		fmt.Printf("❌ Failed to send local player confirmation to %s: %v\n", client.PublicID, err)
		return
	}

	fmt.Printf("🎮 Sent local player confirmation to %s\n", client.PublicID)
}

func (m *LobbyModule) broadcastPlayerJoin(conn net.PacketConn, joiningClient *ClientInfo, lobbyJoin *gamepacket.LobbyJoinBroadcast) {
	remotePacket := &gamepacket.GamePacket{
		Seq: generateRandomSeq(),
		LobbyJoinBroadcast: &gamepacket.LobbyJoinBroadcast{
			PublicId:      lobbyJoin.PublicId,
			Colorhex:      lobbyJoin.Colorhex,
			Position:      lobbyJoin.Position,
			IsLocalPlayer: false,
		},
	}

	BroadcastToAll(conn, remotePacket, joiningClient.PrivateID)
}

func (m *LobbyModule) sendWelcomeSequence(conn net.PacketConn, client *ClientInfo) {
	// Send welcome message
	m.sendWelcomeMessage(conn, client)

	// Send existing lobby members
	m.sendExistingLobbyMembers(conn, client)

	// Send lobby statistics
	m.sendLobbyStatistics(conn, client)
}

func (m *LobbyModule) sendWelcomeMessage(conn net.PacketConn, client *ClientInfo) {
	welcomePacket := &gamepacket.GamePacket{
		Seq: generateRandomSeq(),
		ServerStatus: &gamepacket.ServerStatus{
			Message: fmt.Sprintf("Welcome to the lobby, %s! You are now connected and in the lobby.", client.PublicID),
		},
	}

	if err := SendToClient(conn, welcomePacket, client); err != nil {
		fmt.Printf("❌ Failed to send welcome message to %s: %v\n", client.PublicID, err)
	}
}

func (m *LobbyModule) sendExistingLobbyMembers(conn net.PacketConn, newClient *ClientInfo) {
	allClients := GetAllClients()
	sentCount := 0

	for _, client := range allClients {
		// Skip self and clients not properly in lobby
		if !m.shouldIncludeInLobbyList(client, newClient.PublicID) {
			continue
		}

		// Get position if available
		var lobbyPosition *gamepacket.ClientLobbyPosition
		if pos, hasPosition := GetLobbyPosition(client.PublicID); hasPosition {
			lobbyPosition = &gamepacket.ClientLobbyPosition{
				Position: &gamepacket.Position{
					X: pos.X,
					Y: pos.Y,
					Z: pos.Z,
				},
			}
		}

		// Send existing player info
		packet := &gamepacket.GamePacket{
			Seq: generateRandomSeq(),
			LobbyJoinBroadcast: &gamepacket.LobbyJoinBroadcast{
				PublicId: client.PublicID,
				Colorhex: client.ColorHex,
				Position: lobbyPosition,
			},
		}

		if err := SendToClient(conn, packet, newClient); err != nil {
			fmt.Printf("❌ Failed to send existing player %s to %s: %v\n", client.PublicID, newClient.PublicID, err)
		} else {
			sentCount++
		}
	}

	if sentCount > 0 {
		fmt.Printf("📋 Sent %d existing lobby members to %s\n", sentCount, newClient.PublicID)
	}
}

func (m *LobbyModule) shouldIncludeInLobbyList(client *ClientInfo, excludePublicID string) bool {
	return client.PublicID != excludePublicID &&
		client.ColorHex != "" &&
		client.InLobby
}

func (m *LobbyModule) sendLobbyStatistics(conn net.PacketConn, client *ClientInfo) {
	stats := m.calculateLobbyStats()

	statsMessage := fmt.Sprintf("Lobby Status: %d/%d players in lobby",
		stats.LobbyClients, stats.TotalClients)

	statsPacket := &gamepacket.GamePacket{
		Seq: generateRandomSeq(),
		ServerStatus: &gamepacket.ServerStatus{
			Message: statsMessage,
		},
	}

	if err := SendToClient(conn, statsPacket, client); err != nil {
		fmt.Printf("❌ Failed to send lobby stats to %s: %v\n", client.PublicID, err)
	}
}

// === Statistics and Utilities ===

type LobbyStats struct {
	TotalClients int
	LobbyClients int
}

func (m *LobbyModule) calculateLobbyStats() LobbyStats {
	allClients := GetAllClients()
	stats := LobbyStats{
		TotalClients: len(allClients),
	}

	for _, client := range allClients {
		if client.ColorHex != "" && client.InLobby {
			stats.LobbyClients++
		}
	}

	return stats
}

// getRandomColor provides a fallback color when no colors are available
func getRandomColor() string {
	colors := []string{
		"#FF0000", "#00FF00", "#0000FF", "#FFFF00", "#FF00FF", "#00FFFF",
		"#FFA500", "#800080", "#008000", "#000080", "#800000", "#808000",
	}
	return colors[rand.Intn(len(colors))]
}
