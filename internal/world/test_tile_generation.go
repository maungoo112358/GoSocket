package world

import (
	"fmt"
	"gosocket/gamepacket"
	"math/rand"
	"time"
)

func generateStaticTestTiles() []*gamepacket.Tile {
	return generateRectangularMap(6, 4)
}

func generateRectangularMap(width, height int) []*gamepacket.Tile {
	tiles := make([]*gamepacket.Tile, 0)
	tileSize := float32(20)

	for row := 0; row < height; row++ {
		for col := 0; col < width; col++ {
			x := float32(col) * tileSize
			z := float32(row) * tileSize

			var tileType gamepacket.TileType
			var rotationY float32 = 0
			var scaleX, scaleZ float32 = 1, 1

			if (row == 0 || row == height-1) && (col == 0 || col == width-1) {
				tileType = gamepacket.TileType_CROSS_INTERSECTION_2_WAYS
				if row == 0 && col == 0 {
					rotationY = 90
				} else if row == 0 && col == width-1 {
					rotationY = 0
				} else if row == height-1 && col == 0 {
					rotationY = 180
				} else if row == height-1 && col == width-1 {
					rotationY = 270
				}
			} else if row == 0 || row == height-1 || col == 0 || col == width-1 {
				tileType = gamepacket.TileType_ROAD_LANE
				if row == 0 || row == height-1 {
					rotationY = 90
				}
			} else {
				tileType = gamepacket.TileType_GRASS
				scaleX, scaleZ = 2, 2
			}

			tiles = append(tiles, &gamepacket.Tile{
				TileId:     generateTileID(),
				Position:   &gamepacket.Vector_3{X: x, Y: 0, Z: z},
				Rotation:   &gamepacket.Vector_3{X: 0, Y: rotationY, Z: 0},
				Scale:      &gamepacket.Vector_3{X: scaleX, Y: 1, Z: scaleZ},
				Type:       tileType,
				IsScalable: false,
			})
		}
	}

	fmt.Printf("📦 Generated %dx%d rectangular test map with %d tiles\n", width, height, len(tiles))
	return tiles
}

func generateTileID() string {
	timeStamp := time.Now().UnixNano()
	randNum := uint32(rand.Intn(100000))
	return fmt.Sprintf("tile_%d_%d", timeStamp, randNum)
}
