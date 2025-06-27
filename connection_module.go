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
		m.sendUsernameResponse(conn, addr, pkt.Seq, username, false, "Username must be at least 3 characters!")
		return
	}

	if len(username) > 20 {
		m.sendUsernameResponse(conn, addr, pkt.Seq, username, false, "Username must be 20 characters or less!")
		return
	}

	// Check username availability using new heartbeat-based validation
	if m.isUsernameAvailable(conn, username) {
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
		m.sendUsernameResponse(conn, addr, pkt.Seq, username, true, "Username accepted!")

		fmt.Printf("✅ User registered: %s from %s (ID: %s)\n", username, addr, privateID)
	} else {
		// Username taken - send suggestions
		message := fmt.Sprintf("Username '%s' is already taken. Please choose a different one.", username)
		m.sendUsernameResponse(conn, addr, pkt.Seq, username, false, message)

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

func (m *ConnectionModule) isUsernameAvailable(conn net.PacketConn, username string) bool {
	allClientsMu.Lock()
	defer allClientsMu.Unlock()

	usernameLower := strings.ToLower(username)
	now := time.Now()

	for privateID, client := range allClients {
		if strings.ToLower(client.PublicID) == usernameLower {
			// Check if client is recently active (last heartbeat < 10 seconds)
			if now.Sub(client.LastHeartbeat) < 10*time.Second {
				fmt.Printf("🔒 Username '%s' is taken by active client %s (last heartbeat: %.1fs ago)\n", username, client.PublicID, now.Sub(client.LastHeartbeat).Seconds())
				return false
			} else {
				// Client is stale - remove them and allow new registration
				fmt.Printf("🗑️ Removing stale client %s (last heartbeat: %.1fs ago)\n", client.PublicID, now.Sub(client.LastHeartbeat).Seconds())
				removeClient(conn, privateID)

				fmt.Printf("✨ Username '%s' is now available (stale client removed)\n", username)
				return true
			}
		}
	}

	// Username not found in active clients
	return true
}

func (m *ConnectionModule) sendUsernameResponse(conn net.PacketConn, addr net.Addr, seq uint32, username string, accepted bool, message string) {
	response := &gamepacket.GamePacket{
		Seq: seq,
		UsernameResponse: &gamepacket.UsernameResponse{
			Username:   username,
			IsAccepted: accepted,
			Message:    message,
		},
	}

	if data, err := proto.Marshal(response); err == nil {
		conn.WriteTo(data, addr)
	}
}

func (m *ConnectionModule) tryRestoreClient(username, sessionToken string, addr net.Addr) *ClientInfo {
	allClientsMu.Lock()
	defer allClientsMu.Unlock()

	usernameLower := strings.ToLower(username)
	now := time.Now()

	// Check if client has lobby state
	var hadLobbyState bool
	allClientsLobbyPosMu.RLock()
	_, hadLobbyState = allClientsLobbyPos[username]
	allClientsLobbyPosMu.RUnlock()

	// Check for existing client
	for privateID, client := range allClients {
		if strings.ToLower(client.PublicID) == usernameLower {
			// Same session - just update address and heartbeat
			if client.SessionToken == sessionToken {
				client.Address = addr
				client.LastHeartbeat = now
				fmt.Printf("🔄 Same session reconnecting: %s\n", username)
				return client
			}

			// Different session with stale client
			if now.Sub(client.LastHeartbeat) >= 10*time.Second {
				savedColor := client.ColorHex
				wasInLobby := client.InLobby

				delete(allClients, privateID)
				fmt.Printf("🗑️ Removed stale client for reconnection: %s\n", username)

				// Create new client but KEEP the provided sessionToken
				newPrivateID := generatePrivateID()
				newClient := &ClientInfo{
					PrivateID:     newPrivateID,
					PublicID:      username,
					Name:          username,
					Address:       addr,
					LastHeartbeat: now,
					ConnectedAt:   now,
					SessionToken:  sessionToken, // Use the client's session token
					ColorHex:      savedColor,
					InLobby:       wasInLobby,
				}

				allClients[newPrivateID] = newClient
				return newClient
			} else {
				fmt.Printf("❌ Reconnection denied - username taken by active client: %s\n", username)
				return nil
			}
		}
	}

	// No conflicting client - create new with provided session token
	privateID := generatePrivateID()
	client := &ClientInfo{
		PrivateID:     privateID,
		PublicID:      username,
		Name:          username,
		Address:       addr,
		LastHeartbeat: now,
		ConnectedAt:   now,
		SessionToken:  sessionToken, // Use provided token, don't generate new
		InLobby:       hadLobbyState,
	}

	allClients[privateID] = client
	return client
}
