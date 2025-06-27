package main

import (
	"fmt"
	"gosocket/gamepacket"
	"math"
	"math/rand"
	"net"

	"google.golang.org/protobuf/proto"
)

type LobbyModule struct{}

func NewLobbyModule() *LobbyModule {
	return &LobbyModule{}
}

func (m *LobbyModule) CanHandle(pkt *gamepacket.GamePacket) bool {
	return pkt.GetLobbyJoinBroadcast() != nil
}

func (m *LobbyModule) Handle(conn net.PacketConn, addr net.Addr, pkt *gamepacket.GamePacket) {
	lobby := pkt.GetLobbyJoinBroadcast()
	if lobby == nil {
		return
	}

	fmt.Printf("🔄 LOBBY JOIN: %s with color %s\n", lobby.PublicId, lobby.Colorhex)
	joiningClient := m.findAndUpdateClient(lobby.PublicId)

	if joiningClient == nil {
		fmt.Printf("⚠️ Lobby join from unknown client: %s\n", lobby.PublicId)
		return
	}

	// Mark client as being in lobby - important for disconnect handling
	m.setClientLobbyStatus(joiningClient.PrivateID, true)

	lobby.Colorhex = joiningClient.ColorHex

	// Check if client has existing position (reconnection case)
	allClientsLobbyPosMu.RLock()
	existingPosition, hasExistingPosition := allClientsLobbyPos[joiningClient.PublicID]
	allClientsLobbyPosMu.RUnlock()

	var position LobbyPosition
	if hasExistingPosition {
		// Use existing position (reconnection)
		position = existingPosition
		fmt.Printf("🔄 Restoring %s to previous position (%.2f, %.2f, %.2f)\n",
			joiningClient.PublicID, position.X, position.Y, position.Z)
	} else {
		// Generate new position (first time join)
		position = generateUniquePosition(joiningClient.PublicID)
		fmt.Printf("🎨 %s joined lobby with new position (%.2f, %.2f, %.2f)\n",
			joiningClient.PublicID, position.X, position.Y, position.Z)
	}

	lobby.Position = &gamepacket.ClientLobbyPosition{
		Position: &gamepacket.Position{
			X: position.X,
			Y: position.Y,
			Z: position.Z,
		},
	}

	m.sendToLocalClient(conn, joiningClient, lobby)
	m.sendToRemoteClients(conn, joiningClient, lobby)
	m.sendWelcomeMessage(conn, joiningClient)
	m.sendLobbyStatusToClient(conn, joiningClient)
	m.sendLobbyStats(conn, joiningClient)
}

func (m *LobbyModule) setClientLobbyStatus(privateID string, inLobby bool) {
	allClientsMu.Lock()
	defer allClientsMu.Unlock()

	if client, exists := allClients[privateID]; exists {
		client.InLobby = inLobby
		fmt.Printf("📍 Client %s lobby status updated: %t\n", client.PublicID, inLobby)
	}
}

func (m *LobbyModule) sendToLocalClient(conn net.PacketConn, localClient *ClientInfo, lobby *gamepacket.LobbyJoinBroadcast) {
	localPacket := &gamepacket.GamePacket{
		Seq: uint32(rand.Intn(100000)),
		LobbyJoinBroadcast: &gamepacket.LobbyJoinBroadcast{
			PublicId:      lobby.PublicId,
			Colorhex:      lobby.Colorhex,
			Position:      lobby.Position,
			IsLocalPlayer: true,
		},
	}
	if data, err := proto.Marshal(localPacket); err == nil {
		conn.WriteTo(data, localClient.Address)
		fmt.Printf("🎮 Sent local player confirmation to %s (now in lobby)\n", localClient.PublicID)
	}
}

func (m *LobbyModule) sendToRemoteClients(conn net.PacketConn, remoteClient *ClientInfo, lobby *gamepacket.LobbyJoinBroadcast) {
	remotePacket := &gamepacket.GamePacket{
		Seq: uint32(rand.Intn(100000)),
		LobbyJoinBroadcast: &gamepacket.LobbyJoinBroadcast{
			PublicId:      lobby.PublicId,
			Colorhex:      lobby.Colorhex,
			Position:      lobby.Position,
			IsLocalPlayer: false,
		},
	}
	broadcastToAll(conn, remotePacket, remoteClient.PrivateID)
}

func (m *LobbyModule) findAndUpdateClient(publicID string) *ClientInfo {
	allClientsMu.Lock()
	defer allClientsMu.Unlock()

	fmt.Printf("🔍 Looking for client %s\n", publicID)

	for _, client := range allClients {
		if client.PublicID == publicID {
			fmt.Printf("✅ Found %s, current color: '%s', in lobby: %t\n", publicID, client.ColorHex, client.InLobby)

			// Assign color if not already assigned
			if client.ColorHex == "" {
				fmt.Printf("🎨 Assigning color to %s\n", publicID)

				used := make(map[string]bool)
				for _, c := range allClients {
					if c.ColorHex != "" {
						used[c.ColorHex] = true
					}
				}

				for _, color := range availableColors {
					if !used[color] {
						client.ColorHex = color
						break
					}
				}

				fmt.Printf("🎨 Assigned color %s to %s\n", client.ColorHex, publicID)
			}
			return client
		}
	}
	fmt.Printf("❌ Client %s not found in allClients\n", publicID)
	return nil
}

func (m *LobbyModule) sendWelcomeMessage(conn net.PacketConn, client *ClientInfo) {
	welcomePacket := &gamepacket.GamePacket{
		Seq: uint32(rand.Intn(10000)),
		ServerStatus: &gamepacket.ServerStatus{
			Message: fmt.Sprintf("Welcome to the lobby, %s! You are now connected and in the lobby.", client.PublicID),
		},
	}

	if data, err := proto.Marshal(welcomePacket); err == nil {
		conn.WriteTo(data, client.Address)
	}
}

func (m *LobbyModule) sendLobbyStatusToClient(conn net.PacketConn, newClient *ClientInfo) {
	allClientsMu.RLock()
	allClientsLobbyPosMu.RLock()

	existingPlayersCount := 0

	for _, client := range allClients {
		// Skip self and clients not in lobby
		if client.PublicID == newClient.PublicID || client.ColorHex == "" || !client.InLobby {
			continue
		}

		var lobbyPosition *gamepacket.ClientLobbyPosition
		if position, hasPosition := allClientsLobbyPos[client.PublicID]; hasPosition {
			lobbyPosition = &gamepacket.ClientLobbyPosition{
				Position: &gamepacket.Position{
					X: position.X,
					Y: position.Y,
					Z: position.Z,
				},
			}
		}

		packet := &gamepacket.GamePacket{
			Seq: uint32(rand.Intn(10000)),
			LobbyJoinBroadcast: &gamepacket.LobbyJoinBroadcast{
				PublicId: client.PublicID,
				Colorhex: client.ColorHex,
				Position: lobbyPosition,
			},
		}

		if data, err := proto.Marshal(packet); err == nil {
			conn.WriteTo(data, newClient.Address)
			existingPlayersCount++
		}
	}

	allClientsLobbyPosMu.RUnlock()
	allClientsMu.RUnlock()

	if existingPlayersCount > 0 {
		fmt.Printf("📋 Sent %d existing lobby members to %s\n", existingPlayersCount, newClient.PublicID)
	}
}

func (m *LobbyModule) sendLobbyStats(conn net.PacketConn, client *ClientInfo) {
	allClientsMu.RLock()

	totalClients := len(allClients)
	lobbyClients := 0

	for _, c := range allClients {
		if c.ColorHex != "" && c.InLobby {
			lobbyClients++
		}
	}
	allClientsMu.RUnlock()

	statsMessage := fmt.Sprintf("Lobby Status: %d/%d players in lobby", lobbyClients, totalClients)

	statsPacket := &gamepacket.GamePacket{
		Seq: uint32(rand.Intn(10000)),
		ServerStatus: &gamepacket.ServerStatus{
			Message: statsMessage,
		},
	}

	if data, err := proto.Marshal(statsPacket); err == nil {
		conn.WriteTo(data, client.Address)
	}
}

func generateUniquePosition(id string) LobbyPosition {
	minDist := 5.0
	for minDist >= 0.5 {
		for i := 0; i < 100; i++ {
			x := rand.Float64()*10 - 5
			z := rand.Float64()*10 - 5
			if !HasCollisionWithMinDist(x, 0, z, minDist) {
				pos := LobbyPosition{X: x, Y: 1, Z: z}
				allClientsLobbyPosMu.Lock()
				allClientsLobbyPos[id] = pos
				allClientsLobbyPosMu.Unlock()
				return pos
			}
		}
		minDist -= 0.5
	}
	pos := LobbyPosition{X: 0, Y: 0, Z: 0}
	allClientsLobbyPosMu.Lock()
	allClientsLobbyPos[id] = pos
	allClientsLobbyPosMu.Unlock()
	return pos
}

func HasCollisionWithMinDist(x, y, z, minDist float64) bool {
	allClientsLobbyPosMu.RLock()
	defer allClientsLobbyPosMu.RUnlock()

	for _, other := range allClientsLobbyPos {
		dx := x - other.X
		dy := y - other.Y
		dz := z - other.Z
		if math.Sqrt(dx*dx+dy*dy+dz*dz) < minDist {
			return true
		}
	}
	return false
}

func HasCollision(x, y, z float64) bool {
	allClientsLobbyPosMu.RLock()
	defer allClientsLobbyPosMu.RUnlock()

	for _, other := range allClientsLobbyPos {
		if distance(x, y, z, other.X, other.Y, other.Z) < 5.0 {
			return true
		}
	}
	return false
}

func distance(x1, y1, z1, x2, y2, z2 float64) float64 {
	dx := x1 - x2
	dy := y1 - y2
	dz := z1 - z2
	return math.Sqrt(dx*dx + dy*dy + dz*dz)
}
