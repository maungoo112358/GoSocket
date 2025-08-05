package registry

import (
	"fmt"
	"gosocket/gamepacket"
	"gosocket/internal/collision"
	"gosocket/internal/connection"
	"gosocket/internal/lobby"
	"gosocket/internal/movement"
	"gosocket/internal/world"
	"net"
)

// Module enums
type ModuleEnum int

const (
	ConnectionModuleEnum ModuleEnum = iota
	LobbyModuleEnum
	MovementModuleEnum
	CollisionModuleEnum
	TileGenerationModuleEnum
)

// Module types
type ModuleType int

const (
	Critical ModuleType = iota
	NonCritical
)

// Module metadata
type ModuleInfo struct {
	Name         ModuleEnum
	Type         ModuleType
	Dependencies []ModuleEnum
	SubModules   []ModuleEnum
}

// Tile Generation Service Interface
type TileGenerationService interface {
	GenerateTileForClient(conn net.PacketConn, client interface{})
}

// Module registry
var (
	moduleRegistry = make(map[ModuleEnum]*ModuleInfo)
	enabledModules = make(map[ModuleEnum]bool)
)

func RegisterModule(info ModuleInfo) {
	moduleRegistry[info.Name] = &info
	enabledModules[info.Name] = true
	fmt.Printf("📋 Registered module: %s (Type: %s)\n", getModuleName(info.Name), getModuleTypeName(info.Type))
}

func IsModuleEnabled(module ModuleEnum) bool {
	return enabledModules[module]
}

func AreDependenciesEnabled(module ModuleEnum) bool {
	info, exists := moduleRegistry[module]
	if !exists {
		return true // No info means no dependencies
	}

	for _, dep := range info.Dependencies {
		if !IsModuleEnabled(dep) {
			return false
		}
	}
	return true
}

func DisableModule(module ModuleEnum) {
	enabledModules[module] = false
	fmt.Printf("🔴 Disabled module: %s\n", getModuleName(module))
}

func EnableModule(module ModuleEnum) {
	if !AreDependenciesEnabled(module) {
		fmt.Printf("❌ Cannot enable %s - dependencies not met\n", getModuleName(module))
		return
	}
	enabledModules[module] = true
	fmt.Printf("🟢 Enabled module: %s\n", getModuleName(module))
}

func GetModuleInfo(module ModuleEnum) (*ModuleInfo, bool) {
	info, exists := moduleRegistry[module]
	return info, exists
}

func ListModules() {
	fmt.Println("📋 Module Registry:")
	for moduleEnum, info := range moduleRegistry {
		status := "🔴 Disabled"
		if enabledModules[moduleEnum] {
			status = "🟢 Enabled"
		}
		fmt.Printf("  %s %s (Type: %s)\n", status, getModuleName(moduleEnum), getModuleTypeName(info.Type))

		if len(info.Dependencies) > 0 {
			fmt.Printf("    Dependencies: ")
			for i, dep := range info.Dependencies {
				if i > 0 {
					fmt.Printf(", ")
				}
				fmt.Printf("%s", getModuleName(dep))
			}
			fmt.Println()
		}
	}
}

// Helper functions for readable names
func getModuleName(module ModuleEnum) string {
	switch module {
	case ConnectionModuleEnum:
		return "Connection"
	case LobbyModuleEnum:
		return "Lobby"
	case MovementModuleEnum:
		return "Movement"
	case CollisionModuleEnum:
		return "Collision"
	case TileGenerationModuleEnum:
		return "WorldGeneration"
	default:
		return "Unknown"
	}
}

func getModuleTypeName(moduleType ModuleType) string {
	switch moduleType {
	case Critical:
		return "Critical"
	case NonCritical:
		return "NonCritical"
	default:
		return "Unknown"
	}
}

// Existing ServerModule interface
type ServerModule interface {
	CanHandle(*gamepacket.GamePacket) bool
	Handle(conn net.PacketConn, addr net.Addr, pkt *gamepacket.GamePacket)
	Shutdown()
}

var modules []ServerModule

func init() {
	// Create all module instances
	collisionModule := collision.NewCollisionModule()
	movementModule := movement.NewMovementModule()
	worldGenerationModule := world.NewTileGenerationModule()
	connectionModule := connection.NewConnectionModule()
	lobbyModule := lobby.NewLobbyModule()

	// Register modules with metadata
	RegisterModule(ModuleInfo{
		Name:         CollisionModuleEnum,
		Type:         NonCritical,
		Dependencies: []ModuleEnum{MovementModuleEnum},
		SubModules:   []ModuleEnum{},
	})
	RegisterModule(ModuleInfo{
		Name:         MovementModuleEnum,
		Type:         NonCritical,
		Dependencies: []ModuleEnum{},
		SubModules:   []ModuleEnum{},
	})
	RegisterModule(ModuleInfo{
		Name:         ConnectionModuleEnum,
		Type:         Critical,
		Dependencies: []ModuleEnum{},
		SubModules:   []ModuleEnum{},
	})
	RegisterModule(ModuleInfo{
		Name:         LobbyModuleEnum,
		Type:         Critical,
		Dependencies: []ModuleEnum{},
		SubModules:   []ModuleEnum{},
	})
	RegisterModule(ModuleInfo{
		Name:         TileGenerationModuleEnum,
		Type:         NonCritical,
		Dependencies: []ModuleEnum{},
		SubModules:   []ModuleEnum{},
	})

	// Register services
	RegisterService(CollisionModuleEnum, collisionModule)
	RegisterService(TileGenerationModuleEnum, worldGenerationModule)

	// Wire up dependencies
	movementModule.SetCollisionService(collisionModule)

	// Set up module list for packet dispatching
	modules = []ServerModule{
		connectionModule,
		lobbyModule,
		movementModule,
		worldGenerationModule,
	}

	// Start movement workers
	movementModule.StartWorkers()

	// Print module status after initialization
	fmt.Println("\n🚀 Module initialization complete:")
	ListModules()
	fmt.Println()
}

func DispatchPacket(conn net.PacketConn, addr net.Addr, pkt *gamepacket.GamePacket) {
	for _, m := range modules {
		if m.CanHandle(pkt) {
			m.Handle(conn, addr, pkt)
			return
		}
	}
	fmt.Println("⚠️ No module handled packet")
}

func ShutdownModules() {
	fmt.Println("🛑 Shutting down all modules...")

	for _, module := range modules {
		module.Shutdown()
	}

	fmt.Println("✅ All modules shut down successfully")
}

var services = make(map[ModuleEnum]interface{})

func RegisterService(moduleType ModuleEnum, service interface{}) {
	services[moduleType] = service
	fmt.Printf("🔧 Registered service: %s\n", getModuleName(moduleType))
}

func GetService(moduleType ModuleEnum) interface{} {
	if !IsModuleEnabled(moduleType) {
		return nil
	}
	return services[moduleType]
}
