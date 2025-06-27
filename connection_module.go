package main

import (
	"fmt"
	"gosocket/gamepacket"
	"math/rand"
	"net"
	"strings"
	"time"

	"google.golang.org/protobuf/proto"
)

type ConnectionModule struct{}

func NewConnectionModule() *ConnectionModule {
	return &ConnectionModule{}
}

func (m *ConnectionModule) CanHandle(pkt *gamepacket.GamePacket) bool {
	return pkt.GetHandshakeRequest() != nil ||
		pkt.GetUsernameSubmission() != nil ||
		pkt.GetReconnectionRequest() != nil ||
		pkt.GetHeartbeat() != nil
}

func (m *ConnectionModule) Handle(conn net.PacketConn, addr net.Addr, pkt *gamepacket.GamePacket) {
	switch {
	case pkt.GetHandshakeRequest() != nil:
		m.handleHandshake(conn, addr, pkt)
	case pkt.GetUsernameSubmission() != nil:
		m.handleUsernameSubmission(conn, addr, pkt)
	case pkt.GetReconnectionRequest() != nil:
		m.handleReconnectionRequest(conn, addr, pkt)
	case pkt.GetHeartbeat() != nil:
		m.handleHeartbeat(conn, addr, pkt)
	}
}

func (m *ConnectionModule) handleHandshake(conn net.PacketConn, addr net.Addr, pkt *gamepacket.GamePacket) {
	req := pkt.GetHandshakeRequest()
	if req == nil {
		return
	}

	tempID := generateTempID()
	addPendingClient(tempID, addr)

	// Send handshake acknowledgment with pending IDs
	response := &gamepacket.GamePacket{
		Seq: pkt.Seq,
		HandshakeResponse: &gamepacket.HandshakeResponse{
			PrivateId: "pending",
			PublicId:  "pending",
		},
	}

	if data, err := proto.Marshal(response); err == nil {
		conn.WriteTo(data, addr)
		fmt.Printf("🤝 Handshake acknowledged for %s (temp ID: %s)\n", addr, tempID)
	}

	m.sendUsernamePrompt(conn, addr, "Please enter your username to continue")
}

func (m *ConnectionModule) sendUsernamePrompt(conn net.PacketConn, addr net.Addr, message string) {
	prompt := &gamepacket.GamePacket{
		Seq: uint32(rand.Intn(100000)),
		UsernamePrompt: &gamepacket.UsernamePrompt{
			Message: message,
		},
	}

	if data, err := proto.Marshal(prompt); err == nil {
		conn.WriteTo(data, addr)
		fmt.Printf("📝 Username prompt sent to %s\n", addr)
	}
}

func (m *ConnectionModule) handleUsernameSubmission(conn net.PacketConn, addr net.Addr, pkt *gamepacket.GamePacket) {
	submission := pkt.GetUsernameSubmission()
	if submission == nil {
		return
	}

	username := strings.TrimSpace(submission.Username)
	fmt.Printf("📝 Username submission received: '%s' from %s\n", username, addr)

	if len(username) < 3 {
		m.sendUsernameResponse(conn, addr, pkt.Seq, username, false, "Username must be at least 3 characters!", nil)
		return
	}

	if len(username) > 20 {
		m.sendUsernameResponse(conn, addr, pkt.Seq, username, false, "Username must be 20 characters or less!", nil)
		return
	}

	// Check username availability using new heartbeat-based validation
	if m.isUsernameAvailable(username) {
		// Username is valid - create the client and complete connection
		privateID := generatePrivateID()

		// Remove from pending clients and add to active clients
		removePendingClient(addr)
		addClient(privateID, username, submission.Username, addr)

		// Send final handshake with real IDs
		response := &gamepacket.GamePacket{
			Seq: pkt.Seq,
			HandshakeResponse: &gamepacket.HandshakeResponse{
				PrivateId: privateID,
				PublicId:  username,
			},
		}

		if data, err := proto.Marshal(response); err == nil {
			conn.WriteTo(data, addr)
		}

		// Also send username acceptance confirmation
		m.sendUsernameResponse(conn, addr, pkt.Seq, username, true, "Username accepted!", nil)

		fmt.Printf("✅ User registered: %s from %s (ID: %s)\n", username, addr, privateID)
	} else {
		// Username taken - send suggestions
		suggestions := m.generateUsernameSuggestions(username)
		message := fmt.Sprintf("Username '%s' is already taken. Please choose a different one.", username)
		m.sendUsernameResponse(conn, addr, pkt.Seq, username, false, message, suggestions)

		fmt.Printf("❌ Username '%s' already taken for %s\n", username, addr)
	}
}

func (m *ConnectionModule) handleReconnectionRequest(conn net.PacketConn, addr net.Addr, pkt *gamepacket.GamePacket) {
	req := pkt.GetReconnectionRequest()
	if req == nil {
		return
	}

	if restoredClient := m.tryRestoreClient(req.Username, req.SessionToken, addr); restoredClient != nil {
		response := &gamepacket.GamePacket{
			Seq: pkt.Seq,
			ReconnectionResponse: &gamepacket.ReconnectionResponse{
				IsSuccessful: true,
				Message:      "Successfully reconnected!",
				PrivateId:    restoredClient.PrivateID,
				PublicId:     restoredClient.PublicID,
			},
		}

		if data, err := proto.Marshal(response); err == nil {
			conn.WriteTo(data, addr)
		}

		fmt.Printf("🔄 Client reconnected: %s from %s\n", restoredClient.PublicID, addr)
	} else {
		response := &gamepacket.GamePacket{
			Seq: pkt.Seq,
			ReconnectionResponse: &gamepacket.ReconnectionResponse{
				IsSuccessful: false,
				Message:      "Reconnection failed. Session expired or invalid credentials.",
			},
		}

		if data, err := proto.Marshal(response); err == nil {
			conn.WriteTo(data, addr)
		}

		fmt.Printf("❌ Failed reconnection attempt for %s from %s\n", req.Username, addr)
	}
}

func (m *ConnectionModule) handleHeartbeat(conn net.PacketConn, addr net.Addr, pkt *gamepacket.GamePacket) {
	hb := pkt.GetHeartbeat()
	if hb == nil {
		return
	}

	if !updateHeartbeat(hb.ClientId) {
		fmt.Printf("⚠️ Invalid heartbeat from unknown client: %s\n", hb.ClientId)
		return
	}

	ack := &gamepacket.GamePacket{
		Seq:          pkt.Seq,
		HeartbeatAck: &gamepacket.HeartbeatAck{ClientId: hb.ClientId},
	}

	if data, err := proto.Marshal(ack); err == nil {
		conn.WriteTo(data, addr)
	}
}

// NEW: Heartbeat-based username validation
func (m *ConnectionModule) isUsernameAvailable(username string) bool {
	allClientsMu.Lock()
	defer allClientsMu.Unlock()

	usernameLower := strings.ToLower(username)
	now := time.Now()

	for privateID, client := range allClients {
		if strings.ToLower(client.PublicID) == usernameLower {
			// Check if client is recently active (last heartbeat < 10 seconds)
			if now.Sub(client.LastHeartbeat) < 10*time.Second {
				fmt.Printf("🔒 Username '%s' is taken by active client %s (last heartbeat: %.1fs ago)\n",
					username, client.PublicID, now.Sub(client.LastHeartbeat).Seconds())
				return false
			} else {
				// Client is stale - remove them and allow new registration
				fmt.Printf("🗑️ Removing stale client %s (last heartbeat: %.1fs ago)\n",
					client.PublicID, now.Sub(client.LastHeartbeat).Seconds())

				// Clean up stale client
				delete(allClients, privateID)
				cleanupClientData(client)

				fmt.Printf("✨ Username '%s' is now available (stale client removed)\n", username)
				return true
			}
		}
	}

	// Username not found in active clients
	return true
}

func (m *ConnectionModule) generateUsernameSuggestions(baseUsername string) []string {
	suggestions := make([]string, 0, 3)

	for i := 1; i <= 99; i++ {
		suggestion := fmt.Sprintf("%s%d", baseUsername, i)
		if m.isUsernameAvailable(suggestion) {
			suggestions = append(suggestions, suggestion)
			if len(suggestions) >= 3 {
				break
			}
		}
	}

	suffixes := []string{"_gamer", "_pro", "_player", "_x", "_2024", "_cool"}
	for _, suffix := range suffixes {
		if len(suggestions) >= 3 {
			break
		}
		suggestion := baseUsername + suffix
		if m.isUsernameAvailable(suggestion) {
			suggestions = append(suggestions, suggestion)
		}
	}

	for len(suggestions) < 3 {
		randomNum := rand.Intn(9999) + 1000
		suggestion := fmt.Sprintf("%s_%d", baseUsername, randomNum)
		if m.isUsernameAvailable(suggestion) {
			suggestions = append(suggestions, suggestion)
		}
	}

	return suggestions
}

func (m *ConnectionModule) sendUsernameResponse(conn net.PacketConn, addr net.Addr, seq uint32, username string, accepted bool, message string, suggestions []string) {
	response := &gamepacket.GamePacket{
		Seq: seq,
		UsernameResponse: &gamepacket.UsernameResponse{
			Username:    username,
			IsAccepted:  accepted,
			Message:     message,
			Suggestions: suggestions,
		},
	}

	if data, err := proto.Marshal(response); err == nil {
		conn.WriteTo(data, addr)
	}
}

func (m *ConnectionModule) tryRestoreClient(username, sessionToken string, addr net.Addr) *ClientInfo {
	// For reconnection, we need to be more careful about preserving position and lobby state
	allClientsMu.Lock()
	defer allClientsMu.Unlock()

	usernameLower := strings.ToLower(username)
	now := time.Now()

	// First, save any existing position data for this username
	var savedPosition *LobbyPosition
	var hadLobbyState bool

	allClientsLobbyPosMu.RLock()
	if pos, exists := allClientsLobbyPos[username]; exists {
		savedPosition = &LobbyPosition{X: pos.X, Y: pos.Y, Z: pos.Z}
		hadLobbyState = true
		fmt.Printf("💾 Found saved position for %s: (%.2f, %.2f, %.2f)\n", username, pos.X, pos.Y, pos.Z)
	}
	allClientsLobbyPosMu.RUnlock()

	// Check if there's an existing client with this username
	for privateID, client := range allClients {
		if strings.ToLower(client.PublicID) == usernameLower {
			// If it's the same session trying to reconnect, allow it
			if client.SessionToken == sessionToken {
				// Update address and heartbeat but preserve all other data
				client.Address = addr
				client.LastHeartbeat = now
				fmt.Printf("🔄 Same session reconnecting: %s (preserving state)\n", username)
				return client
			}

			// Different session - check if existing client is stale
			if now.Sub(client.LastHeartbeat) >= 10*time.Second {
				// Save color and lobby state before removing stale client
				savedColor := client.ColorHex
				wasInLobby := client.InLobby

				// Remove stale client but don't clean up position data yet
				delete(allClients, privateID)
				fmt.Printf("🗑️ Removed stale client for reconnection: %s (preserving position data)\n", username)

				// Create new client with preserved data
				newPrivateID := generatePrivateID()
				newSessionToken := generateSessionToken()

				newClient := &ClientInfo{
					PrivateID:     newPrivateID,
					PublicID:      username,
					Name:          username,
					Address:       addr,
					LastHeartbeat: now,
					ConnectedAt:   now,
					SessionToken:  newSessionToken,
					ColorHex:      savedColor, // Preserve color
					InLobby:       wasInLobby, // Preserve lobby state
				}

				allClients[newPrivateID] = newClient

				// Position data is already preserved in allClientsLobbyPos map
				if savedPosition != nil {
					fmt.Printf("🎨 Restored client %s with color %s and position (%.2f, %.2f, %.2f)\n",
						username, savedColor, savedPosition.X, savedPosition.Y, savedPosition.Z)
				}

				return newClient
			} else {
				// Active client with different session - deny reconnection
				fmt.Printf("❌ Reconnection denied - username taken by active client: %s\n", username)
				return nil
			}
		}
	}

	// No conflicting client found - create new client for reconnection
	// This handles case where client was properly cleaned up but position data remains
	privateID := generatePrivateID()
	sessionTokenNew := generateSessionToken()

	client := &ClientInfo{
		PrivateID:     privateID,
		PublicID:      username,
		Name:          username,
		Address:       addr,
		LastHeartbeat: now,
		ConnectedAt:   now,
		SessionToken:  sessionTokenNew,
		InLobby:       hadLobbyState, // Restore lobby state if position existed
	}

	allClients[privateID] = client

	if savedPosition != nil {
		fmt.Printf("✅ Created new client for reconnection: %s with restored position (%.2f, %.2f, %.2f)\n",
			username, savedPosition.X, savedPosition.Y, savedPosition.Z)
	} else {
		fmt.Printf("✅ Created new client for reconnection: %s (no previous position)\n", username)
	}

	return client
}
