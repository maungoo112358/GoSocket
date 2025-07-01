package main

import (
	"gosocket/gamepacket"
	"net"
)

// Pure service interface
type CollisionService interface {
	CheckPlayerCollision(clientID string, position *gamepacket.Vector_3) bool
	CheckBuildingCollision(buildingID string, position *gamepacket.Vector_3) bool
	BroadcastRejection(conn net.PacketConn, clientID string, seq uint32)
}
