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

	colorPair, hasUniqueColors := getAvailableColorPair()
	var headColor, bodyColor string
	if hasUniqueColors {
		headColor = colorPair.Head
		bodyColor = colorPair.Body
	} else {
		headColor = availableColors[rand.Intn(len(availableColors))]
		bodyColor = availableColors[rand.Intn(len(availableColors))]
		for bodyColor == headColor {
			bodyColor = availableColors[rand.Intn(len(availableColors))]
		}
	}

	// Find the client who is joining
	allClientsMu.Lock()
	var joiningClient *ClientInfo

	for _, client := range allClients {
		if client.PublicID == lobby.PublicId {
			client.ColorHex_Head = headColor
			client.ColorHex = bodyColor
			joiningClient = client
			break
		}
	}

	allClientsMu.Unlock()

	if joiningClient == nil {
		fmt.Printf("⚠️ Lobby join from unknown client: %s\n", lobby.PublicId)
		return
	}

	fmt.Printf("🎨 %s joined lobby with color %s\n", lobby.PublicId, lobby.Colorhex)

	go generateUniquePosition(joiningClient.PublicID)

	m.sendWelcomeMessage(conn, joiningClient)

	m.sendLobbyStatusToClient(conn, joiningClient)

	position := generateUniquePosition(joiningClient.PublicID)
	lobbyPosition := &gamepacket.ClientLobbyPosition{
		X: position.X,
		Y: position.Y,
		Z: position.Z,
	}
	lobby.Position = lobbyPosition

	broadcastToAll(conn, pkt, "")

	m.sendLobbyStats(conn, joiningClient)
}

// Send welcome message to the joining client
func (m *LobbyModule) sendWelcomeMessage(conn net.PacketConn, client *ClientInfo) {
	welcomePacket := &gamepacket.GamePacket{
		Seq: uint32(rand.Intn(10000)),
		ServerStatus: &gamepacket.ServerStatus{
			Message: fmt.Sprintf("Welcome to the lobby, %s! You are now connected.", client.PublicID),
		},
	}

	if data, err := proto.Marshal(welcomePacket); err == nil {
		conn.WriteTo(data, client.Address)
	}
}

// Send current lobby members to the newly joined client
func (m *LobbyModule) sendLobbyStatusToClient(conn net.PacketConn, newClient *ClientInfo) {
	allClientsMu.RLock()
	defer allClientsMu.RUnlock()

	existingPlayersCount := 0

	// Send info about each existing player to the new client
	for _, client := range allClients {
		// Skip the new client themselves and clients without lobby color (haven't joined lobby yet)
		if client.PublicID != newClient.PublicID && client.ColorHex != "" && client.ColorHex_Head != "" {

			// Get existing player's position
			allClientsLobbyPosMu.RLock()
			position, hasPosition := allClientsLobbyPos[client.PublicID]
			allClientsLobbyPosMu.RUnlock()

			var lobbyPosition *gamepacket.ClientLobbyPosition
			if hasPosition {
				lobbyPosition = &gamepacket.ClientLobbyPosition{
					X: position.X,
					Y: position.Y,
					Z: position.Z,
				}
			}

			// Create a lobby join broadcast for each existing player
			existingPlayerPacket := &gamepacket.GamePacket{
				Seq: uint32(rand.Intn(10000)),
				LobbyJoinBroadcast: &gamepacket.LobbyJoinBroadcast{
					PublicId:     client.PublicID,
					Colorhex:     client.ColorHex,
					ColorhexHead: client.ColorHex_Head,
					Position:     lobbyPosition,
				},
			}

			if data, err := proto.Marshal(existingPlayerPacket); err == nil {
				conn.WriteTo(data, newClient.Address)
				existingPlayersCount++
			}
		}
	}

	if existingPlayersCount > 0 {
		fmt.Printf("📋 Sent %d existing lobby members to %s\n", existingPlayersCount, newClient.PublicID)
	}
}

// Send lobby statistics to the client
func (m *LobbyModule) sendLobbyStats(conn net.PacketConn, client *ClientInfo) {
	allClientsMu.RLock()

	totalClients := len(allClients)
	lobbyClients := 0

	// Count clients who have joined the lobby (have a color)
	for _, c := range allClients {
		if c.ColorHex != "" {
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
