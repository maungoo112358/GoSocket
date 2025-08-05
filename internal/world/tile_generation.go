package world

import (
	"fmt"
	"gosocket/gamepacket"
	"gosocket/internal/client"
	"gosocket/internal/utils"
	"math/rand"
	"net"
	"time"
)

type TileType int

const (
	NONE         TileType = iota
	ROAD_LANE_NS          // North-South road
	ROAD_LANE_EW          // East-West road
	CROSS_INTERSECTION
	GRASS
)

// For adjacency checking
type Direction int

const (
	NORTH Direction = iota
	SOUTH
	EAST
	WEST
)

// Tile properties for WFC logic
type TileInfo struct {
	TileType    TileType
	LogicalSize int                // 1 for grass, 2 for roads/intersections
	Connections map[Direction]bool // which sides can connect
}

type WFCCell struct {
	Possibilities map[TileType]bool
	IsCollapsed   bool
	FinalTile     TileType
	Entropy       int // number of possibilities remaining
}

type GridPos struct {
	X, Y int
}

type WFCGrid struct {
	Width  int
	Height int
	Cells  [][]WFCCell
}

// defines which tiles can be neighbors
type AdjacencyRules map[TileType]map[Direction][]TileType

type TileGenerationModule struct{}

var allTileTypes = []TileType{GRASS, ROAD_LANE_NS, ROAD_LANE_EW, CROSS_INTERSECTION}

var tileProperties map[TileType]TileInfo
var adjacencyRules AdjacencyRules

func NewTileGenerationModule() *TileGenerationModule {
	service := &TileGenerationModule{}

	tileProperties = initializeTileProperties()
	adjacencyRules = initializeAdjacencyRules()

	return service
}

func (m *TileGenerationModule) CanHandle(pkt *gamepacket.GamePacket) bool {
	return false
}

func (m *TileGenerationModule) Handle(conn net.PacketConn, addr net.Addr, pkt *gamepacket.GamePacket) {
	// Not used since CanHandle returns false
}

func (m *TileGenerationModule) GenerateTileForClient(conn net.PacketConn, c interface{}) {
	clientInfo, ok := c.(*client.ClientInfo)
	if !ok {
		fmt.Printf("❌ Invalid client type for tile generation\n")
		return
	}

	fmt.Printf("🌍 Generating world for client: %s\n", clientInfo.PublicID)

	// Generate static test tiles
	tiles := generateStaticTestTiles()

	packet := &gamepacket.GamePacket{
		Seq: utils.GenerateRandomSeq(),
		TileSet: &gamepacket.TileSet{
			Tiles:     tiles,
			WorldSize: int32(len(tiles)),
		},
	}

	if err := client.SendToClient(conn, packet, clientInfo); err != nil {
		fmt.Printf("❌ Failed to send world tiles to %s: %v\n", clientInfo.PublicID, err)
	} else {
		fmt.Printf("🌍 Sent %d tiles to %s\n", len(tiles), clientInfo.PublicID)
	}
}

func initializeTileProperties() map[TileType]TileInfo {
	tileProperties := make(map[TileType]TileInfo)

	tileProperties[GRASS] = TileInfo{
		TileType:    GRASS,
		LogicalSize: 1,
		Connections: map[Direction]bool{
			NORTH: true,
			SOUTH: true,
			EAST:  true,
			WEST:  true,
		},
	}

	tileProperties[ROAD_LANE_NS] = TileInfo{
		TileType:    ROAD_LANE_NS,
		LogicalSize: 2,
		Connections: map[Direction]bool{
			NORTH: true,
			SOUTH: true,
			EAST:  false,
			WEST:  false,
		},
	}

	tileProperties[ROAD_LANE_EW] = TileInfo{
		TileType:    ROAD_LANE_EW,
		LogicalSize: 2,
		Connections: map[Direction]bool{
			NORTH: false,
			SOUTH: false,
			EAST:  true,
			WEST:  true,
		},
	}

	tileProperties[CROSS_INTERSECTION] = TileInfo{
		TileType:    CROSS_INTERSECTION,
		LogicalSize: 2,
		Connections: map[Direction]bool{
			NORTH: true,
			SOUTH: true,
			EAST:  true,
			WEST:  true,
		},
	}

	return tileProperties
}

func initializeAdjacencyRules() AdjacencyRules {
	rules := make(AdjacencyRules)

	// GRASS - can connect to anything in all directions
	rules[GRASS] = map[Direction][]TileType{
		NORTH: {GRASS, ROAD_LANE_NS, ROAD_LANE_EW, CROSS_INTERSECTION},
		SOUTH: {GRASS, ROAD_LANE_NS, ROAD_LANE_EW, CROSS_INTERSECTION},
		EAST:  {GRASS, ROAD_LANE_NS, ROAD_LANE_EW, CROSS_INTERSECTION},
		WEST:  {GRASS, ROAD_LANE_NS, ROAD_LANE_EW, CROSS_INTERSECTION},
	}

	// ROAD_LANE_NS - connects only north/south to roads/intersections
	rules[ROAD_LANE_NS] = map[Direction][]TileType{
		NORTH: {ROAD_LANE_NS, CROSS_INTERSECTION, GRASS},
		SOUTH: {ROAD_LANE_NS, CROSS_INTERSECTION, GRASS},
		EAST:  {GRASS}, // Only grass can connect from the sides
		WEST:  {GRASS},
	}

	// ROAD_LANE_EW - connects only east/west to roads/intersections
	rules[ROAD_LANE_EW] = map[Direction][]TileType{
		NORTH: {GRASS}, // Only grass can connect from top/bottom
		SOUTH: {GRASS},
		EAST:  {ROAD_LANE_EW, CROSS_INTERSECTION, GRASS},
		WEST:  {ROAD_LANE_EW, CROSS_INTERSECTION, GRASS},
	}

	// CROSS_INTERSECTION - connects to roads/intersections in all directions
	rules[CROSS_INTERSECTION] = map[Direction][]TileType{
		NORTH: {ROAD_LANE_NS, CROSS_INTERSECTION, GRASS},
		SOUTH: {ROAD_LANE_NS, CROSS_INTERSECTION, GRASS},
		EAST:  {ROAD_LANE_EW, CROSS_INTERSECTION, GRASS},
		WEST:  {ROAD_LANE_EW, CROSS_INTERSECTION, GRASS},
	}
	return rules
}

func createWFCGrid(width, height int) *WFCGrid {
	cells := make([][]WFCCell, height)
	for y := 0; y < height; y++ {
		cells[y] = make([]WFCCell, width)
		for x := 0; x < width; x++ {
			cells[y][x] = initializeCell()
		}
	}

	return &WFCGrid{
		Width:  width,
		Height: height,
		Cells:  cells,
	}
}

func initializeCell() WFCCell {
	possibilities := make(map[TileType]bool)

	for _, tiletype := range getAllTileTypes() {
		possibilities[tiletype] = true
	}

	return WFCCell{
		Possibilities: possibilities,
		IsCollapsed:   false,
		FinalTile:     NONE,
		Entropy:       len(possibilities),
	}
}

func canPlaceTileAt(grid *WFCGrid, x, y int, tileType TileType) bool {
	tileInfo, exists := tileProperties[tileType]
	if !exists {
		return false
	}

	size := tileInfo.LogicalSize

	for dy := 0; dy < size; dy++ {
		for dx := 0; dx < size; dx++ {
			checkX := x + dx
			checkY := y + dy

			if !isValidPosition(grid, checkX, checkY) {
				return false
			}

			cell := grid.Cells[checkY][checkX]
			if !cell.Possibilities[tileType] {
				return false
			}
		}
	}
	return true

}

func isValidPosition(grid *WFCGrid, x, y int) bool {
	return x >= 0 && x < grid.Width && y >= 0 && y < grid.Height
}

func getOppositeDirection(dir Direction) Direction {
	return dir ^ 1
}

// Edge bounds are not checked here—they're handled in another function.
func getNeighbourPos(x, y int) []GridPos {
	return []GridPos{
		{X: x, Y: y - 1}, //NORTH
		{X: x, Y: y + 1}, //SOUTH
		{X: x + 1, Y: y}, //EAST
		{X: x - 1, Y: y}, //WEST
	}
}

func calculateEntropy(cell *WFCCell) int {
	count := 0

	for _, isPossible := range cell.Possibilities {
		if isPossible {
			count++
		}
	}
	return count
}

func getValidNeighborTiles(tileType TileType, direction Direction) []TileType {
	var validTiles []TileType

	for _, candidateTile := range getAllTileTypes() {
		if canTileConnect(tileType, candidateTile, direction) {
			validTiles = append(validTiles, candidateTile)
		}
	}

	return validTiles
}

func getAllTileTypes() []TileType {
	return []TileType{GRASS, ROAD_LANE_NS, ROAD_LANE_EW, CROSS_INTERSECTION}
}

func canTileConnect(tile1, tile2 TileType, direction Direction) bool {
	info1, exist1 := tileProperties[tile1]
	info2, exist2 := tileProperties[tile2]

	if !exist1 || !exist2 {
		return false
	}

	if !info1.Connections[direction] {
		return false
	}

	oppositeDirection := getOppositeDirection(direction)
	if !info2.Connections[oppositeDirection] {
		return false
	}
	return true
}

func addToQueue(queue *[]GridPos, pos GridPos) {

	for _, existing := range *queue {
		if existing.X == pos.X && existing.Y == pos.Y {
			return
		}
	}

	*queue = append(*queue, pos)
}

func propagateConstraints(grid *WFCGrid, startX, startY int) bool {
	var queue []GridPos

	neighbours := getNeighbourPos(startX, startY)
	for _, neighbour := range neighbours {
		if isValidPosition(grid, neighbour.X, neighbour.Y) {
			addToQueue(&queue, neighbour)
		}
	}

	for len(queue) > 0 {
		currentPos := queue[0]
		queue = queue[1:] // Remove first element

		success := updateCellConstraints(grid, currentPos.X, currentPos.Y)
		if !success {
			return false
		}

		cell := &grid.Cells[currentPos.Y][currentPos.X]
		if cell.Entropy == 1 && !cell.IsCollapsed {
			cellNeighbours := getNeighbourPos(currentPos.X, currentPos.Y)
			for _, neighbour := range cellNeighbours {
				if isValidPosition(grid, neighbour.X, neighbour.Y) {
					addToQueue(&queue, neighbour)
				}
			}
		}
	}
	return true
}

func updateCellConstraints(grid *WFCGrid, x, y int) bool {
	cell := &grid.Cells[y][x]

	if cell.IsCollapsed {
		return true
	}

	neighbours := getNeighbourPos(x, y)
	directions := []Direction{NORTH, SOUTH, EAST, WEST}

	for i, neighboursPos := range neighbours {
		if !isValidPosition(grid, neighboursPos.X, neighboursPos.Y) {
			continue
		}

		neighbourCell := &grid.Cells[neighboursPos.Y][neighboursPos.X]

		if !neighbourCell.IsCollapsed {
			continue
		}

		direction := directions[i]
		neighbourTile := neighbourCell.FinalTile

		for tileType := range cell.Possibilities {

			if !cell.Possibilities[tileType] {
				continue
			}

			if !canTileConnect(tileType, neighbourTile, direction) {
				if !removePossibilities(cell, tileType) {
					return false
				}
			}
		}
	}
	return true
}

var contradictionDetected bool

func resetContradictionFlag() {
	contradictionDetected = false
}

func hasContradictionFast() bool {
	return contradictionDetected
}

func removePossibilities(cell *WFCCell, tileType TileType) bool {

	if !cell.Possibilities[tileType] {
		return true
	}

	cell.Possibilities[tileType] = false

	cell.Entropy = calculateEntropy(cell)

	if cell.Entropy == 0 {
		contradictionDetected = true
		return false // Contradiction detected
	}

	return true
}

func generateStaticTestTiles() []*gamepacket.Tile {
	tiles := make([]*gamepacket.Tile, 0)

	tiles = append(tiles, &gamepacket.Tile{
		TileId:     generateTileID(),
		Position:   &gamepacket.Vector_3{X: 0, Y: 0, Z: 0},
		Rotation:   &gamepacket.Vector_3{X: 0, Y: 0, Z: 0},
		Scale:      &gamepacket.Vector_3{X: 1, Y: 1, Z: 1},
		Type:       gamepacket.TileType_CROSS_INTERSECTION,
		IsScalable: false,
	})

	tiles = append(tiles, &gamepacket.Tile{
		TileId:     generateTileID(),
		Position:   &gamepacket.Vector_3{X: 20, Y: 0, Z: 0},
		Rotation:   &gamepacket.Vector_3{X: 0, Y: 0, Z: 0},
		Scale:      &gamepacket.Vector_3{X: 1, Y: 1, Z: 1},
		Type:       gamepacket.TileType_ROAD_LANE,
		IsScalable: false,
	})

	tiles = append(tiles, &gamepacket.Tile{
		TileId:     generateTileID(),
		Position:   &gamepacket.Vector_3{X: 10, Y: 0, Z: 15},
		Rotation:   &gamepacket.Vector_3{X: 0, Y: 0, Z: 0},
		Scale:      &gamepacket.Vector_3{X: 1, Y: 1, Z: 1},
		Type:       gamepacket.TileType_GRASS,
		IsScalable: false,
	})

	fmt.Printf("📦 Generated %d static test tiles\n", len(tiles))
	return tiles
}

func generateTileID() string {
	timeStamp := time.Now().UnixNano()
	randNum := uint32(rand.Intn(100000))

	return fmt.Sprintf("tile_%d_%d", timeStamp, randNum)
}

func (m *TileGenerationModule) Shutdown() {
}
