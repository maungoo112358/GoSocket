package main

import (
	"fmt"
	"gosocket/gamepacket"
	"math"
	"net"
	"time"
)

const (
	ClientRadius      = 0.5
	CollisionGridSize = 10.0
)

type CollisionModule struct {
	name    string
	enabled bool
}

func NewCollisionModule() *CollisionModule {
	RegisterModule(ModuleInfo{
		Name:         CollisionModuleEnum,
		Type:         NonCritical,
		Dependencies: []ModuleEnum{MovementModuleEnum}, // Add this
		SubModules:   []ModuleEnum{},
	})

	service := &CollisionModule{
		name:    "CollisionModule",
		enabled: IsModuleEnabled(MovementModuleEnum), // Base on movement
	}

	RegisterService(CollisionModuleEnum, service) // Add this
	return service
}

func (c *CollisionModule) CheckPlayerCollision(clientID string, position *gamepacket.Position) bool {
	if !c.enabled {
		return false
	}

	allPositions := GetAllLobbyPositions()
	logFlag := false
	playersInGrid := c.getPlayersInGrid(position, allPositions, clientID, &logFlag)

	for _, playerPos := range playersInGrid {
		if c.hasCollision(position, playerPos) {
			return true
		}
	}
	return false
}

func (c *CollisionModule) CheckBuildingCollision(buildingID string, position *gamepacket.Position) bool {
	return false // will add later
}

func (m *CollisionModule) getPlayersInGrid(centerPos *gamepacket.Position, allPositions map[string]LobbyPosition, excludeClientID string, isLog *bool) map[string]LobbyPosition {
	gridHalfSize := CollisionGridSize / 2.0
	gridMinX := centerPos.X - gridHalfSize
	gridMaxX := centerPos.X + gridHalfSize
	gridMinZ := centerPos.Z - gridHalfSize
	gridMaxZ := centerPos.Z + gridHalfSize

	playersInGrid := make(map[string]LobbyPosition)

	for playerID, playerPos := range allPositions {
		if playerID == excludeClientID {
			continue
		}

		if m.isPositionInGrid(playerPos, gridMinX, gridMaxX, gridMinZ, gridMaxZ) {
			playersInGrid[playerID] = playerPos
		}
	}

	shouldLog := true
	if isLog != nil {
		shouldLog = *isLog
	}
	if shouldLog {
		fmt.Printf("🔍 Checking collision for %s: %d players in grid\n", excludeClientID, len(playersInGrid))
	}

	return playersInGrid
}

func (m *CollisionModule) isPositionInGrid(pos LobbyPosition, minX, maxX, minZ, maxZ float64) bool {
	return pos.X >= minX && pos.X <= maxX && pos.Z >= minZ && pos.Z <= maxZ
}

func (m *CollisionModule) hasCollision(pos1 *gamepacket.Position, pos2 LobbyPosition) bool {
	dx := pos1.X - pos2.X
	dz := pos1.Z - pos2.Z
	distance := math.Sqrt(dx*dx + dz*dz)

	minDistance := ClientRadius + ClientRadius
	return distance < minDistance
}

func (m *CollisionModule) BroadcastRejection(conn net.PacketConn, clientID string, seq uint32) {
	currentPos, exists := GetLobbyPosition(clientID)
	if !exists {
		fmt.Printf("⚠️ Cannot broadcast position - no position found for %s\n", clientID)
		return
	}

	correctionPacket := &gamepacket.GamePacket{
		Seq: seq,
		ClientPosition: &gamepacket.ClientPosition{
			ClientId: clientID,
			Position: &gamepacket.Position{
				X: currentPos.X,
				Y: currentPos.Y,
				Z: currentPos.Z,
			},
			Timestamp: float32(time.Now().UnixMilli()) / 1000.0,
		},
	}

	BroadcastToAll(conn, correctionPacket, "")
	fmt.Printf("🚫 Collision rejected: broadcasted current position for %s to all clients\n", clientID)
}
