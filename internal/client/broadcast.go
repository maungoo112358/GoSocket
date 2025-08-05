package client

import (
	"fmt"
	"gosocket/gamepacket"
	"gosocket/internal/utils"
	"math/rand"
	"net"
	"sync"
	"sync/atomic"

	"google.golang.org/protobuf/proto"
)

func BroadcastToAll(conn net.PacketConn, packet *gamepacket.GamePacket, excludePrivateID string) {
	data, err := marshalPacket(packet, "broadcast")
	if err != nil {
		return
	}

	targets := getTargetClients(excludePrivateID)
	if len(targets) == 0 {
		return
	}

	sendConcurrently(conn, data, targets, nil)
}

func BroadcastToAllExcept(conn net.PacketConn, packet *gamepacket.GamePacket, excludeClientID string, isLog *bool) {
	data, err := marshalPacket(packet, "movement")
	if err != nil {
		return
	}

	targets := getTargetClientsByPublicID(excludeClientID)
	if len(targets) == 0 {
		return
	}

	sendConcurrently(conn, data, targets, isLog)
}

func SendToClient(conn net.PacketConn, packet *gamepacket.GamePacket, clientInfo *ClientInfo) error {
	data, err := marshalPacket(packet, "client")
	if err != nil {
		return err
	}

	_, err = conn.WriteTo(data, clientInfo.Address)
	if err != nil {
		return fmt.Errorf("failed to send to %s: %w", clientInfo.PublicID, err)
	}

	return nil
}

func BroadcastToLobbyClients(conn net.PacketConn, packet *gamepacket.GamePacket, excludePrivateID string) {
	data, err := marshalPacket(packet, "lobby broadcast")
	if err != nil {
		return
	}

	targets := getLobbyTargetClients(excludePrivateID)
	if len(targets) == 0 {
		return
	}

	sendConcurrently(conn, data, targets, nil)
	fmt.Printf("📡 Lobby broadcast sent to %d clients\n", len(targets))
}

func BroadcastPlayerLeft(conn net.PacketConn, client *ClientInfo) {
	leavePacket := &gamepacket.GamePacket{
		Seq: uint32(rand.Intn(10000)),
		ServerStatus: &gamepacket.ServerStatus{
			Message:  fmt.Sprintf("Player %s left the lobby", client.PublicID),
			ClientId: client.PublicID,
		},
	}

	BroadcastToAll(conn, leavePacket, client.PrivateID)
	fmt.Printf("📤 Broadcast: %s left the lobby\n", client.PublicID)
}

// === Helper Functions ===

func getTargetClients(excludePrivateID string) []*ClientInfo {
	allClients := GetAllClients()

	targets := make([]*ClientInfo, 0, len(allClients))
	for _, c := range allClients {
		if c.PrivateID != excludePrivateID {
			targets = append(targets, c)
		}
	}

	return targets
}

func getTargetClientsByPublicID(excludePublicID string) []*ClientInfo {
	allClients := GetAllClients()

	targets := make([]*ClientInfo, 0, len(allClients))
	for _, c := range allClients {
		if c.PublicID != excludePublicID {
			targets = append(targets, c)
		}
	}

	return targets
}

func getLobbyTargetClients(excludePrivateID string) []*ClientInfo {
	allClients := GetAllClients()

	targets := make([]*ClientInfo, 0)
	for _, c := range allClients {
		if c.PrivateID != excludePrivateID && c.InLobby && c.ColorHex != "" {
			targets = append(targets, c)
		}
	}

	return targets
}

func sendConcurrently(conn net.PacketConn, data []byte, targets []*ClientInfo, isLog *bool) {
	var wg sync.WaitGroup
	sentCount := int32(0)

	for _, c := range targets {
		wg.Add(1)
		go func(cl *ClientInfo) {
			defer wg.Done()

			if _, err := conn.WriteTo(data, cl.Address); err == nil {
				atomic.AddInt32(&sentCount, 1)
			} else {
				fmt.Printf("⚠️ Failed to send to %s (%s): %v\n", cl.PublicID, cl.Address, err)
			}
		}(c)
	}

	wg.Wait()
	
	if isLog == nil || *isLog {
		if len(targets) > 0 {
			fmt.Printf("📡 Broadcast sent to %d/%d clients\n", sentCount, len(targets))
		}
	}
}

func marshalPacket(packet *gamepacket.GamePacket, context string) ([]byte, error) {
	data, err := proto.Marshal(packet)
	if err != nil {
		fmt.Printf("❌ Failed to marshal %s packet: %v\n", context, err)
		return nil, err
	}
	return data, nil
}

// === Utility Functions for Specific Broadcasts ===

func BroadcastServerMessage(conn net.PacketConn, message string) {
	packet := &gamepacket.GamePacket{
		Seq: utils.GenerateRandomSeq(),
		ServerStatus: &gamepacket.ServerStatus{
			Message: message,
		},
	}

	BroadcastToAll(conn, packet, "")
	fmt.Printf("📢 Server message broadcast: %s\n", message)
}

func BroadcastPlayerJoined(conn net.PacketConn, joinedClient *ClientInfo, lobbyJoinData *gamepacket.LobbyJoinBroadcast) {
	packet := &gamepacket.GamePacket{
		Seq: utils.GenerateRandomSeq(),
		LobbyJoinBroadcast: &gamepacket.LobbyJoinBroadcast{
			PublicId:      lobbyJoinData.PublicId,
			Colorhex:      lobbyJoinData.Colorhex,
			Position:      lobbyJoinData.Position,
			IsLocalPlayer: false, // Always false for other players
		},
	}

	BroadcastToAll(conn, packet, joinedClient.PrivateID)
	fmt.Printf("📡 Player join broadcast: %s\n", joinedClient.PublicID)
}
