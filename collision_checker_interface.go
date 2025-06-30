package main

import (
	"gosocket/gamepacket"
	"net"
)

// Pure service interface
type CollisionService interface {
	CheckPlayerCollision(clientID string, position *gamepacket.Position) bool
	CheckBuildingCollision(buildingID string, position *gamepacket.Position) bool
	BroadcastRejection(conn net.PacketConn, clientID string, seq uint32)
}
