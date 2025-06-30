package main

import (
	"fmt"
	"gosocket/gamepacket"
	"net"
	"strings"
	"time"
)

const (
	MaxUsernameLength  = 20
	MinUsernameLength  = 3
	StaleClientTimeout = 10 * time.Second
)

type ConnectionModule struct{}

func NewConnectionModule() *ConnectionModule {
	RegisterModule(ModuleInfo{
		Name:         ConnectionModuleEnum,
		Type:         Critical,
		Dependencies: []ModuleEnum{},
		SubModules:   []ModuleEnum{},
	})
	return &ConnectionModule{}
}

// CanHandle determines if this module can process the packet
func (m *ConnectionModule) CanHandle(pkt *gamepacket.GamePacket) bool {
	return pkt.GetHandshakeRequest() != nil ||
		pkt.GetUsernameSubmission() != nil ||
		pkt.GetReconnectionRequest() != nil ||
		pkt.GetHeartbeat() != nil
}

// Handle routes the packet to the appropriate handler
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

// === Handshake Flow ===

func (m *ConnectionModule) handleHandshake(conn net.PacketConn, addr net.Addr, pkt *gamepacket.GamePacket) {
	req := pkt.GetHandshakeRequest()
	if req == nil {
		return
	}

	// Generate temp ID and register pending client
	tempID := GenerateTempID()
	AddPendingClient(tempID, addr)

	// Send initial handshake response with pending status
	response := &gamepacket.GamePacket{
		Seq: pkt.Seq,
		HandshakeResponse: &gamepacket.HandshakeResponse{
			PrivateId: "pending",
			PublicId:  "pending",
		},
	}

	if err := SendToAddress(conn, response, addr); err != nil {
		fmt.Printf("❌ Failed to send handshake response to %s: %v\n", addr, err)
		return
	}

	// Prompt for username
	m.sendUsernamePrompt(conn, addr, "Please enter your username to continue")

	fmt.Printf("🤝 Handshake acknowledged for %s (temp ID: %s)\n", addr, tempID)
}

func (m *ConnectionModule) sendUsernamePrompt(conn net.PacketConn, addr net.Addr, message string) {
	prompt := &gamepacket.GamePacket{
		Seq: generateRandomSeq(),
		UsernamePrompt: &gamepacket.UsernamePrompt{
			Message: message,
		},
	}

	if err := SendToAddress(conn, prompt, addr); err != nil {
		fmt.Printf("❌ Failed to send username prompt to %s: %v\n", addr, err)
		return
	}

	fmt.Printf("📝 Username prompt sent to %s\n", addr)
}

// === Username Submission ===

func (m *ConnectionModule) handleUsernameSubmission(conn net.PacketConn, addr net.Addr, pkt *gamepacket.GamePacket) {
	submission := pkt.GetUsernameSubmission()
	if submission == nil {
		return
	}

	username := strings.TrimSpace(submission.Username)
	fmt.Printf("📝 Username submission: '%s' from %s\n", username, addr)

	// Validate username format
	if valid, errorMsg := IsValidUsername(username); !valid {
		m.sendUsernameResponse(conn, addr, pkt.Seq, username, false, errorMsg)
		return
	}

	// Check availability and handle accordingly
	if m.isUsernameAvailable(conn, username) {
		m.acceptUsername(conn, addr, pkt.Seq, username, submission.Username)
	} else {
		m.rejectUsername(conn, addr, pkt.Seq, username)
	}
}

func (m *ConnectionModule) acceptUsername(conn net.PacketConn, addr net.Addr, seq uint32, username, originalUsername string) {
	// Generate client IDs
	privateID := GeneratePrivateID()

	// Clean up pending client
	RemovePendingClient(addr)

	// Register new active client
	AddClient(privateID, username, originalUsername, addr)

	// Send final handshake with real IDs
	handshakeResponse := &gamepacket.GamePacket{
		Seq: seq,
		HandshakeResponse: &gamepacket.HandshakeResponse{
			PrivateId: privateID,
			PublicId:  username,
		},
	}

	if err := SendToAddress(conn, handshakeResponse, addr); err != nil {
		fmt.Printf("❌ Failed to send final handshake to %s: %v\n", addr, err)
	}

	// Send username acceptance confirmation
	m.sendUsernameResponse(conn, addr, seq, username, true, "Username accepted!")

	fmt.Printf("✅ User registered: %s from %s (ID: %s)\n", username, addr, privateID)
}

func (m *ConnectionModule) rejectUsername(conn net.PacketConn, addr net.Addr, seq uint32, username string) {
	message := fmt.Sprintf("Username '%s' is already taken. Please choose a different one.", username)
	m.sendUsernameResponse(conn, addr, seq, username, false, message)
	fmt.Printf("❌ Username '%s' rejected for %s\n", username, addr)
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

	if err := SendToAddress(conn, response, addr); err != nil {
		fmt.Printf("❌ Failed to send username response to %s: %v\n", addr, err)
	}
}

// === Username Availability Logic ===

func (m *ConnectionModule) isUsernameAvailable(conn net.PacketConn, username string) bool {
	existingClient := FindClientByPublicID(username)
	if existingClient == nil {
		return true // Username not found, available
	}

	// Username exists - check if client is stale
	if m.isClientStale(existingClient) {
		m.removeStaleClient(conn, existingClient, username)
		return true
	}

	// Active client has this username
	fmt.Printf("🔒 Username '%s' taken by active client (last heartbeat: %.1fs ago)\n",
		username, time.Since(existingClient.LastHeartbeat).Seconds())
	return false
}

func (m *ConnectionModule) isClientStale(client *ClientInfo) bool {
	return time.Since(client.LastHeartbeat) >= StaleClientTimeout
}

func (m *ConnectionModule) removeStaleClient(conn net.PacketConn, client *ClientInfo, username string) {
	timeSinceHeartbeat := time.Since(client.LastHeartbeat)

	fmt.Printf("🗑️ Removing stale client %s (last heartbeat: %.1fs ago)\n",
		client.PublicID, timeSinceHeartbeat.Seconds())

	RemoveClient(conn, client.PrivateID)

	fmt.Printf("✨ Username '%s' now available (stale client removed)\n", username)
}

// === Reconnection Flow ===

func (m *ConnectionModule) handleReconnectionRequest(conn net.PacketConn, addr net.Addr, pkt *gamepacket.GamePacket) {
	req := pkt.GetReconnectionRequest()
	if req == nil {
		return
	}

	fmt.Printf("🔄 Reconnection attempt: %s from %s\n", req.Username, addr)

	if restoredClient := m.tryRestoreClient(req.Username, req.SessionToken, addr); restoredClient != nil {
		m.sendReconnectionSuccess(conn, addr, pkt.Seq, restoredClient)
		fmt.Printf("🔄 Client reconnected: %s from %s\n", restoredClient.PublicID, addr)
	} else {
		m.sendReconnectionFailure(conn, addr, pkt.Seq, req.Username)
		fmt.Printf("❌ Reconnection failed for %s from %s\n", req.Username, addr)
	}
}

func (m *ConnectionModule) sendReconnectionSuccess(conn net.PacketConn, addr net.Addr, seq uint32, client *ClientInfo) {
	response := &gamepacket.GamePacket{
		Seq: seq,
		ReconnectionResponse: &gamepacket.ReconnectionResponse{
			IsSuccessful: true,
			Message:      "Successfully reconnected!",
			PrivateId:    client.PrivateID,
			PublicId:     client.PublicID,
		},
	}

	if err := SendToAddress(conn, response, addr); err != nil {
		fmt.Printf("❌ Failed to send reconnection success to %s: %v\n", addr, err)
	}
}

func (m *ConnectionModule) sendReconnectionFailure(conn net.PacketConn, addr net.Addr, seq uint32, username string) {
	response := &gamepacket.GamePacket{
		Seq: seq,
		ReconnectionResponse: &gamepacket.ReconnectionResponse{
			IsSuccessful: false,
			Message:      "Reconnection failed. Session expired or invalid credentials.",
		},
	}

	if err := SendToAddress(conn, response, addr); err != nil {
		fmt.Printf("❌ Failed to send reconnection failure to %s: %v\n", addr, err)
	}
}

// === Client Restoration Logic ===

func (m *ConnectionModule) tryRestoreClient(username, sessionToken string, addr net.Addr) *ClientInfo {
	existingClient := FindClientByPublicID(username)

	// Check if client has preserved lobby state
	_, hadLobbyState := GetLobbyPosition(username)

	if existingClient != nil {
		return m.handleExistingClientReconnection(existingClient, sessionToken, addr, username)
	}

	// No existing client - create new one if they had lobby state
	if hadLobbyState {
		return m.createRestoredClient(username, sessionToken, addr, true)
	}

	fmt.Printf("❌ No existing client or lobby state for %s\n", username)
	return nil
}

func (m *ConnectionModule) handleExistingClientReconnection(existing *ClientInfo, sessionToken string, addr net.Addr, username string) *ClientInfo {
	// Same session - just update connection info
	if existing.SessionToken == sessionToken {
		existing.Address = addr
		existing.LastHeartbeat = time.Now()
		fmt.Printf("🔄 Same session reconnecting: %s\n", username)
		return existing
	}

	// Different session - check if old client is stale
	if m.isClientStale(existing) {
		return m.replaceStaleClient(existing, sessionToken, addr, username)
	}

	fmt.Printf("❌ Reconnection denied - username taken by active client: %s\n", username)
	return nil
}

func (m *ConnectionModule) replaceStaleClient(existing *ClientInfo, sessionToken string, addr net.Addr, username string) *ClientInfo {
	// Save important state before removal
	savedColor := existing.ColorHex
	wasInLobby := existing.InLobby

	// Remove old client
	RemoveClient(nil, existing.PrivateID) // Don't broadcast since we're replacing
	fmt.Printf("🗑️ Removed stale client for reconnection: %s\n", username)

	// Create new client with preserved state
	newClient := m.createRestoredClient(username, sessionToken, addr, wasInLobby)
	if newClient != nil && savedColor != "" {
		newClient.ColorHex = savedColor
	}

	return newClient
}

func (m *ConnectionModule) createRestoredClient(username, sessionToken string, addr net.Addr, inLobby bool) *ClientInfo {
	privateID := GeneratePrivateID()
	now := time.Now()

	client := &ClientInfo{
		PrivateID:     privateID,
		PublicID:      username,
		Name:          username,
		Address:       addr,
		LastHeartbeat: now,
		ConnectedAt:   now,
		SessionToken:  sessionToken,
		InLobby:       inLobby,
	}

	// Register the client (this will be handled by client_manager)
	// For now, we'll need to add this to the client manager
	AddClientDirect(client)

	return client
}

// === Heartbeat Handling ===

func (m *ConnectionModule) handleHeartbeat(conn net.PacketConn, addr net.Addr, pkt *gamepacket.GamePacket) {
	hb := pkt.GetHeartbeat()
	if hb == nil {
		return
	}

	if !UpdateHeartbeat(hb.ClientId) {
		fmt.Printf("⚠️ Invalid heartbeat from unknown client: %s\n", hb.ClientId)
		return
	}

	// Send heartbeat acknowledgment
	ack := &gamepacket.GamePacket{
		Seq:          pkt.Seq,
		HeartbeatAck: &gamepacket.HeartbeatAck{ClientId: hb.ClientId},
	}

	if err := SendToAddress(conn, ack, addr); err != nil {
		fmt.Printf("❌ Failed to send heartbeat ack to %s: %v\n", addr, err)
	}
}
