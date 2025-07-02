package main

import (
	"fmt"
	"gosocket/gamepacket"
	"math/rand"
	"net"
	"time"
)

type TileGenerationModule struct{}

func NewTileGenerationModule() *TileGenerationModule {
	RegisterModule(ModuleInfo{
		Name:         TileGenerationModuleEnum,
		Type:         NonCritical,
		Dependencies: []ModuleEnum{},
		SubModules:   []ModuleEnum{},
	})

	service := &TileGenerationModule{}
	RegisterService(TileGenerationModuleEnum, service)

	return service
}

func (m *TileGenerationModule) CanHandle(pkt *gamepacket.GamePacket) bool {
	return false
}

func (m *TileGenerationModule) Handle(conn net.PacketConn, addr net.Addr, pkt *gamepacket.GamePacket) {
	// Not used since CanHandle returns false
}

func (m *TileGenerationModule) GenerateTileForClient(conn net.PacketConn, client *ClientInfo) {
	fmt.Printf("🌍 Generating Tile for client: %s\n", client.PublicID)

	// Create 3 basic tiles
	tiles := []*gamepacket.Tile{
		{
			TileId:     generateTileId(),
			Position:   &gamepacket.Vector_3{X: 0, Y: 0, Z: 0},
			Rotation:   &gamepacket.Vector_3{X: 0, Y: 0, Z: 0},
			Scale:      &gamepacket.Vector_3{X: 1, Y: 1, Z: 1},
			Type:       gamepacket.TileType_ROAD_LANE,
			IsScalable: false,
		},
		{
			TileId:     generateTileId(),
			Position:   &gamepacket.Vector_3{X: 1, Y: 0, Z: 0},
			Rotation:   &gamepacket.Vector_3{X: 0, Y: 0, Z: 0},
			Scale:      &gamepacket.Vector_3{X: 1, Y: 1, Z: 1},
			Type:       gamepacket.TileType_CROSS_SECTION,
			IsScalable: false,
		},
		{
			TileId:     generateTileId(),
			Position:   &gamepacket.Vector_3{X: 2, Y: 0, Z: 0},
			Rotation:   &gamepacket.Vector_3{X: 0, Y: 0, Z: 0},
			Scale:      &gamepacket.Vector_3{X: 1, Y: 1, Z: 1},
			Type:       gamepacket.TileType_GRASS,
			IsScalable: false,
		},
	}

	packet := &gamepacket.GamePacket{
		Seq: generateRandomSeq(),
		TileSet: &gamepacket.TileSet{
			Tiles:     tiles,
			WorldSize: 3,
		},
	}

	if err := SendToClient(conn, packet, client); err != nil {
		fmt.Printf("❌ Failed to send tile generation message to %s: %v\n", client.PublicID, err)
	}
}

func generateTileId() string {
	timeStamp := time.Now().UnixNano()
	randNum := uint32(rand.Intn(100000))

	return fmt.Sprintf("tile_%d_%d", timeStamp, randNum)

}
