package world

type TileType int

const (
	NONE         TileType = iota
	ROAD_LANE_NS          
	ROAD_LANE_EW          
	CROSS_INTERSECTION_2_WAYS
	CROSS_INTERSECTION_3_WAYS
	CROSS_INTERSECTION_4_WAYS
	GRASS
)

type Direction int

const (
	NORTH Direction = iota
	SOUTH
	EAST
	WEST
)

type TileInfo struct {
	TileType    TileType
	LogicalSize int
	Connections map[Direction]bool
}

func initializeTileProperties() map[TileType]TileInfo {
	tileProperties := make(map[TileType]TileInfo)

	// can connect to anything
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

	// only connects N/S to maintain traffic flow
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

	//  only connects E/W to maintain traffic flow
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

	//  L-shaped corner intersection, connects on 2 sides
	tileProperties[CROSS_INTERSECTION_2_WAYS] = TileInfo{
		TileType:    CROSS_INTERSECTION_2_WAYS,
		LogicalSize: 2,
		Connections: map[Direction]bool{
			NORTH: true,
			SOUTH: true,
			EAST:  true,
			WEST:  true,
		},
	}

	//  T-junction intersection, connects on 3 sides
	tileProperties[CROSS_INTERSECTION_3_WAYS] = TileInfo{
		TileType:    CROSS_INTERSECTION_3_WAYS,
		LogicalSize: 2,
		Connections: map[Direction]bool{
			NORTH: true,
			SOUTH: true,
			EAST:  true,
			WEST:  true,
		},
	}

	// Full 4-way intersection, connects in all directions
	tileProperties[CROSS_INTERSECTION_4_WAYS] = TileInfo{
		TileType:    CROSS_INTERSECTION_4_WAYS,
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

	// can be placed next to any tile type
	rules[GRASS] = map[Direction][]TileType{
		NORTH: {GRASS, ROAD_LANE_NS, ROAD_LANE_EW, CROSS_INTERSECTION_2_WAYS, CROSS_INTERSECTION_3_WAYS, CROSS_INTERSECTION_4_WAYS},
		SOUTH: {GRASS, ROAD_LANE_NS, ROAD_LANE_EW, CROSS_INTERSECTION_2_WAYS, CROSS_INTERSECTION_3_WAYS, CROSS_INTERSECTION_4_WAYS},
		EAST:  {GRASS, ROAD_LANE_NS, ROAD_LANE_EW, CROSS_INTERSECTION_2_WAYS, CROSS_INTERSECTION_3_WAYS, CROSS_INTERSECTION_4_WAYS},
		WEST:  {GRASS, ROAD_LANE_NS, ROAD_LANE_EW, CROSS_INTERSECTION_2_WAYS, CROSS_INTERSECTION_3_WAYS, CROSS_INTERSECTION_4_WAYS},
	}

	// Vertical road lane, connects N/S to roads and intersections only
	rules[ROAD_LANE_NS] = map[Direction][]TileType{
		NORTH: {ROAD_LANE_NS, CROSS_INTERSECTION_2_WAYS, CROSS_INTERSECTION_3_WAYS, CROSS_INTERSECTION_4_WAYS, GRASS},
		SOUTH: {ROAD_LANE_NS, CROSS_INTERSECTION_2_WAYS, CROSS_INTERSECTION_3_WAYS, CROSS_INTERSECTION_4_WAYS, GRASS},
		EAST:  {GRASS}, // Only grass can connect from the sides to maintain road integrity
		WEST:  {GRASS},
	}

	//  Horizontal road lane, connects E/W to roads and intersections only
	rules[ROAD_LANE_EW] = map[Direction][]TileType{
		NORTH: {GRASS}, // Only grass can connect from top/bottom to maintain road integrity
		SOUTH: {GRASS},
		EAST:  {ROAD_LANE_EW, CROSS_INTERSECTION_2_WAYS, CROSS_INTERSECTION_3_WAYS, CROSS_INTERSECTION_4_WAYS, GRASS},
		WEST:  {ROAD_LANE_EW, CROSS_INTERSECTION_2_WAYS, CROSS_INTERSECTION_3_WAYS, CROSS_INTERSECTION_4_WAYS, GRASS},
	}

	// L-shaped corner intersection, connects to roads on 2 sides
	rules[CROSS_INTERSECTION_2_WAYS] = map[Direction][]TileType{
		NORTH: {ROAD_LANE_NS, CROSS_INTERSECTION_2_WAYS, CROSS_INTERSECTION_3_WAYS, CROSS_INTERSECTION_4_WAYS, GRASS},
		SOUTH: {ROAD_LANE_NS, CROSS_INTERSECTION_2_WAYS, CROSS_INTERSECTION_3_WAYS, CROSS_INTERSECTION_4_WAYS, GRASS},
		EAST:  {ROAD_LANE_EW, CROSS_INTERSECTION_2_WAYS, CROSS_INTERSECTION_3_WAYS, CROSS_INTERSECTION_4_WAYS, GRASS},
		WEST:  {ROAD_LANE_EW, CROSS_INTERSECTION_2_WAYS, CROSS_INTERSECTION_3_WAYS, CROSS_INTERSECTION_4_WAYS, GRASS},
	}

	// T-junction intersection, connects to roads on 3 sides
	rules[CROSS_INTERSECTION_3_WAYS] = map[Direction][]TileType{
		NORTH: {ROAD_LANE_NS, CROSS_INTERSECTION_2_WAYS, CROSS_INTERSECTION_3_WAYS, CROSS_INTERSECTION_4_WAYS, GRASS},
		SOUTH: {ROAD_LANE_NS, CROSS_INTERSECTION_2_WAYS, CROSS_INTERSECTION_3_WAYS, CROSS_INTERSECTION_4_WAYS, GRASS},
		EAST:  {ROAD_LANE_EW, CROSS_INTERSECTION_2_WAYS, CROSS_INTERSECTION_3_WAYS, CROSS_INTERSECTION_4_WAYS, GRASS},
		WEST:  {ROAD_LANE_EW, CROSS_INTERSECTION_2_WAYS, CROSS_INTERSECTION_3_WAYS, CROSS_INTERSECTION_4_WAYS, GRASS},
	}

	//  Full intersection, connects to all road types in all directions
	rules[CROSS_INTERSECTION_4_WAYS] = map[Direction][]TileType{
		NORTH: {ROAD_LANE_NS, CROSS_INTERSECTION_2_WAYS, CROSS_INTERSECTION_3_WAYS, CROSS_INTERSECTION_4_WAYS, GRASS},
		SOUTH: {ROAD_LANE_NS, CROSS_INTERSECTION_2_WAYS, CROSS_INTERSECTION_3_WAYS, CROSS_INTERSECTION_4_WAYS, GRASS},
		EAST:  {ROAD_LANE_EW, CROSS_INTERSECTION_2_WAYS, CROSS_INTERSECTION_3_WAYS, CROSS_INTERSECTION_4_WAYS, GRASS},
		WEST:  {ROAD_LANE_EW, CROSS_INTERSECTION_2_WAYS, CROSS_INTERSECTION_3_WAYS, CROSS_INTERSECTION_4_WAYS, GRASS},
	}

	return rules
}

func getAllTileTypes() []TileType {
	return []TileType{
		GRASS,
		ROAD_LANE_NS,
		ROAD_LANE_EW,
		CROSS_INTERSECTION_2_WAYS,
		CROSS_INTERSECTION_3_WAYS,
		CROSS_INTERSECTION_4_WAYS,
	}
}
