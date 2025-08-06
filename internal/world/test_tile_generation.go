package world

import (
	"fmt"
	"gosocket/gamepacket"
	"math/rand"
	"time"
)

func generateStaticTestTiles() []*gamepacket.Tile {
	return generateThreeLaneGridMap()
}

func generateThreeLaneGridMap() []*gamepacket.Tile {
	tiles := make([]*gamepacket.Tile, 0)
	tileSize := float32(20)
	width, height := 13, 9

	for row := 0; row < height; row++ {
		for col := 0; col < width; col++ {
			x := float32(col) * tileSize
			z := float32(row) * tileSize

			var tileType gamepacket.TileType
			var rotationY float32 = 0
			var scaleX, scaleZ float32 = 1, 1

			if isCorner(row, col, width, height) {
				tileType = gamepacket.TileType_CROSS_INTERSECTION_2_WAYS
				rotationY = getCornerRotation(row, col, width, height)
			} else if isEdgeIntersection(row, col, width, height) {
				tileType = gamepacket.TileType_CROSS_INTERSECTION_3_WAYS
				rotationY = getEdgeIntersectionRotation(row, col, width, height)
			} else if isInternalIntersection(row, col) {
				tileType = gamepacket.TileType_CROSS_INTERSECTION_4_WAYS
			} else if isRoadPosition(row, col) {
				tileType = gamepacket.TileType_ROAD_LANE
				rotationY = getRoadRotation(row, col)
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

	fmt.Printf("📦 Generated 4x3-lane grid map with %d tiles\n", len(tiles))
	return tiles
}

func isCorner(row, col, width, height int) bool {
	return (row == 0 || row == height-1) && (col == 0 || col == width-1)
}

func isEdgeIntersection(row, col, width, height int) bool {
	return (row == 0 || row == height-1) && (col == 4 || col == 8) ||
		(col == 0 || col == width-1) && (row == 4)
}

func isInternalIntersection(row, col int) bool {
	return row == 4 && (col == 4 || col == 8)
}

func isRoadPosition(row, col int) bool {
	// 4-lane horizontal roads between intersections
	horizontalRoads := (row == 0 || row == 4 || row == 8) && 
		((col >= 1 && col <= 3) || (col >= 5 && col <= 7) || (col >= 9 && col <= 11))
	
	// 3-lane vertical roads between intersections  
	verticalRoads := (col == 0 || col == 4 || col == 8 || col == 12) && 
		((row >= 1 && row <= 3) || (row >= 5 && row <= 7))
	
	return horizontalRoads || verticalRoads
}

func getCornerRotation(row, col, width, height int) float32 {
	if row == 0 && col == 0 {
		return 90
	} else if row == 0 && col == width-1 {
		return 0
	} else if row == height-1 && col == 0 {
		return 180
	} else if row == height-1 && col == width-1 {
		return 270
	}
	return 0
}

func getEdgeIntersectionRotation(row, col, width, height int) float32 {
	if row == 0 {
		return 0
	} else if row == height-1 {
		return 180
	} else if col == 0 {
		return 270
	} else if col == width-1 {
		return 90
	}
	return 0
}

func getRoadRotation(row, col int) float32 {
	// Horizontal roads (running east-west) need 90 degree rotation
	if row == 0 || row == 4 || row == 8 {
		return 90
	}
	// Vertical roads (running north-south) use default 0 rotation
	return 0
}

func generateTileID() string {
	timeStamp := time.Now().UnixNano()
	randNum := uint32(rand.Intn(100000))
	return fmt.Sprintf("tile_%d_%d", timeStamp, randNum)
}
