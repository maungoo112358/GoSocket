package lobby

import (
	"fmt"
	"gosocket/gamepacket"
	"gosocket/internal/client"
	"gosocket/internal/utils"
	"gosocket/internal/world"
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
	return &LobbyModule{}
}

func (m *LobbyModule) CanHandle(pkt *gamepacket.GamePacket) bool {
	return pkt.GetLobbyJoinBroadcast() != nil
}

func (m *LobbyModule) Handle(conn net.PacketConn, addr net.Addr, pkt *gamepacket.GamePacket) {
	lobbyJoin := pkt.GetLobbyJoinBroadcast()
	if lobbyJoin == nil {
		return
	}

	fmt.Printf("🔄 LOBBY JOIN: %s with color %s\n", lobbyJoin.PublicId, lobbyJoin.Colorhex)

	joiningClient := m.prepareJoiningClient(lobbyJoin.PublicId)
	if joiningClient == nil {
		fmt.Printf("⚠️ Lobby join from unknown client: %s\n", lobbyJoin.PublicId)
		return
	}

	position := m.getOrCreatePosition(joiningClient)

	m.updateLobbyJoinData(lobbyJoin, joiningClient, position)

	m.handleLobbyJoinNotifications(conn, joiningClient, lobbyJoin)
}

// === Client Preparation ===

func (m *LobbyModule) prepareJoiningClient(publicID string) *client.ClientInfo {
	c := client.FindClientByPublicID(publicID)
	if c == nil {
		return nil
	}

	fmt.Printf("🔍 Found client %s, current color: '%s', in lobby: %t\n",
		publicID, c.ColorHex, c.InLobby)

	client.SetClientLobbyStatus(c.PrivateID, true)

	m.ensureClientHasColor(c)

	return c
}

func (m *LobbyModule) ensureClientHasColor(c *client.ClientInfo) {
	if c.ColorHex != "" {
		return
	}

	color, available := client.GetAvailableColor()
	if !available {
		fmt.Printf("⚠️ No available colors for %s, using random color\n", c.PublicID)
		color = utils.GetRandomColor()
	}

	c.ColorHex = color
	fmt.Printf("🎨 Assigned color %s to %s\n", color, c.PublicID)
}

// === Position Management ===

func (m *LobbyModule) getOrCreatePosition(c *client.ClientInfo) client.LobbyPosition {
	if existingPos, hasPosition := client.GetLobbyPosition(c.PublicID); hasPosition {
		fmt.Printf("🔄 Restoring %s to previous position (%.2f, %.2f, %.2f)\n",
			c.PublicID, existingPos.X, existingPos.Y, existingPos.Z)
		return existingPos
	}

	newPos := m.generateUniquePosition(c.PublicID)
	fmt.Printf("🎨 %s joined lobby with new position (%.2f, %.2f, %.2f)\n",
		c.PublicID, newPos.X, newPos.Y, newPos.Z)

	return newPos
}

func (m *LobbyModule) generateUniquePosition(publicID string) client.LobbyPosition {
	minDistance := float32(MinPositionDistance)

	for minDistance >= 0.5 {
		for attempt := 0; attempt < MaxPositionRetries; attempt++ {
			position := m.generateRandomPosition()

			if !m.hasPositionCollision(position, minDistance) {
				client.SetLobbyPosition(publicID, position)
				return position
			}
		}
		minDistance -= 0.5
	}

	fallbackPos := client.LobbyPosition{X: 0, Y: DefaultLobbyY, Z: 0}
	client.SetLobbyPosition(publicID, fallbackPos)
	fmt.Printf("⚠️ Using fallback position for %s\n", publicID)
	return fallbackPos
}

func (m *LobbyModule) generateRandomPosition() client.LobbyPosition {
	return client.LobbyPosition{
		X: float32((rand.Float64()*2 - 1) * float64(LobbyBoundary)),
		Y: DefaultLobbyY,
		Z: float32((rand.Float64()*2 - 1) * float64(LobbyBoundary)),
	}
}

func (m *LobbyModule) hasPositionCollision(newPos client.LobbyPosition, minDistance float32) bool {
	allPositions := client.GetAllLobbyPositions()

	for _, existingPos := range allPositions {
		distance := m.calculateDistance(newPos, existingPos)
		if distance < minDistance {
			return true
		}
	}

	return false
}

func (m *LobbyModule) calculateDistance(pos1, pos2 client.LobbyPosition) float32 {
	dx := float64(pos1.X - pos2.X)
	dy := float64(pos1.Y - pos2.Y)
	dz := float64(pos1.Z - pos2.Z)
	return float32(math.Sqrt(dx*dx + dy*dy + dz*dz))
}

// === Lobby Join Data Management ===

func (m *LobbyModule) updateLobbyJoinData(lobbyJoin *gamepacket.LobbyJoinBroadcast, c *client.ClientInfo, position client.LobbyPosition) {
	lobbyJoin.Colorhex = c.ColorHex

	lobbyJoin.Position = &gamepacket.ClientLobbyPosition{
		Position: &gamepacket.Vector_3{
			X: position.X,
			Y: position.Y,
			Z: position.Z,
		},
	}
}

// === Notification System ===
func (m *LobbyModule) handleLobbyJoinNotifications(conn net.PacketConn, joiningClient *client.ClientInfo, lobbyJoin *gamepacket.LobbyJoinBroadcast) {
	m.sendLocalPlayerConfirmation(conn, joiningClient, lobbyJoin)

	m.broadcastPlayerJoin(conn, joiningClient, lobbyJoin)

	m.sendWelcomeSequence(conn, joiningClient)
}

func (m *LobbyModule) sendLocalPlayerConfirmation(conn net.PacketConn, c *client.ClientInfo, lobbyJoin *gamepacket.LobbyJoinBroadcast) {
	localPacket := &gamepacket.GamePacket{
		Seq: utils.GenerateRandomSeq(),
		LobbyJoinBroadcast: &gamepacket.LobbyJoinBroadcast{
			PublicId:      lobbyJoin.PublicId,
			Colorhex:      lobbyJoin.Colorhex,
			Position:      lobbyJoin.Position,
			IsLocalPlayer: true,
		},
	}

	if err := client.SendToClient(conn, localPacket, c); err != nil {
		fmt.Printf("❌ Failed to send local player confirmation to %s: %v\n", c.PublicID, err)
		return
	}

	fmt.Printf("🎮 Sent local player confirmation to %s\n", c.PublicID)
}

func (m *LobbyModule) broadcastPlayerJoin(conn net.PacketConn, joiningClient *client.ClientInfo, lobbyJoin *gamepacket.LobbyJoinBroadcast) {
	remotePacket := &gamepacket.GamePacket{
		Seq: utils.GenerateRandomSeq(),
		LobbyJoinBroadcast: &gamepacket.LobbyJoinBroadcast{
			PublicId:      lobbyJoin.PublicId,
			Colorhex:      lobbyJoin.Colorhex,
			Position:      lobbyJoin.Position,
			IsLocalPlayer: false,
		},
	}

	client.BroadcastToAll(conn, remotePacket, joiningClient.PrivateID)
}

func (m *LobbyModule) sendWelcomeSequence(conn net.PacketConn, c *client.ClientInfo) {
	m.sendWelcomeMessage(conn, c)

	m.sendExistingLobbyMembers(conn, c)

	m.sendLobbyStatistics(conn, c)

	tileModule := world.NewTileGenerationModule()
	tileModule.GenerateTileForClient(conn, c)
}

func (m *LobbyModule) sendWelcomeMessage(conn net.PacketConn, c *client.ClientInfo) {
	welcomePacket := &gamepacket.GamePacket{
		Seq: utils.GenerateRandomSeq(),
		ServerStatus: &gamepacket.ServerStatus{
			Message: fmt.Sprintf("Welcome to the lobby, %s! You are now connected and in the lobby.", c.PublicID),
		},
	}

	if err := client.SendToClient(conn, welcomePacket, c); err != nil {
		fmt.Printf("❌ Failed to send welcome message to %s: %v\n", c.PublicID, err)
	}
}

func (m *LobbyModule) sendExistingLobbyMembers(conn net.PacketConn, newClient *client.ClientInfo) {
	allClients := client.GetAllClients()
	sentCount := 0

	for _, c := range allClients {
		// Skip self and clients not properly in lobby
		if !m.shouldIncludeInLobbyList(c, newClient.PublicID) {
			continue
		}

		var lobbyPosition *gamepacket.ClientLobbyPosition
		if pos, hasPosition := client.GetLobbyPosition(c.PublicID); hasPosition {
			lobbyPosition = &gamepacket.ClientLobbyPosition{
				Position: &gamepacket.Vector_3{
					X: pos.X,
					Y: pos.Y,
					Z: pos.Z,
				},
			}
		}

		packet := &gamepacket.GamePacket{
			Seq: utils.GenerateRandomSeq(),
			LobbyJoinBroadcast: &gamepacket.LobbyJoinBroadcast{
				PublicId: c.PublicID,
				Colorhex: c.ColorHex,
				Position: lobbyPosition,
			},
		}

		if err := client.SendToClient(conn, packet, newClient); err != nil {
			fmt.Printf("❌ Failed to send existing player %s to %s: %v\n", c.PublicID, newClient.PublicID, err)
		} else {
			sentCount++
		}
	}

	if sentCount > 0 {
		fmt.Printf("📋 Sent %d existing lobby members to %s\n", sentCount, newClient.PublicID)
	}
}

func (m *LobbyModule) shouldIncludeInLobbyList(c *client.ClientInfo, excludePublicID string) bool {
	return c.PublicID != excludePublicID &&
		c.ColorHex != "" &&
		c.InLobby
}

func (m *LobbyModule) sendLobbyStatistics(conn net.PacketConn, c *client.ClientInfo) {
	stats := m.calculateLobbyStats()

	statsMessage := fmt.Sprintf("Lobby Status: %d/%d players in lobby",
		stats.LobbyClients, stats.TotalClients)

	statsPacket := &gamepacket.GamePacket{
		Seq: utils.GenerateRandomSeq(),
		ServerStatus: &gamepacket.ServerStatus{
			Message: statsMessage,
		},
	}

	if err := client.SendToClient(conn, statsPacket, c); err != nil {
		fmt.Printf("❌ Failed to send lobby stats to %s: %v\n", c.PublicID, err)
	}
}

// === Statistics and Utilities ===

type LobbyStats struct {
	TotalClients int
	LobbyClients int
}

func (m *LobbyModule) calculateLobbyStats() LobbyStats {
	allClients := client.GetAllClients()
	stats := LobbyStats{
		TotalClients: len(allClients),
	}

	for _, c := range allClients {
		if c.ColorHex != "" && c.InLobby {
			stats.LobbyClients++
		}
	}

	return stats
}

func (m *LobbyModule) Shutdown() {
	// Lobby module has no specific shutdown logic
}
