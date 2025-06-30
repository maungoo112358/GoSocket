package main

import (
	"fmt"
	"gosocket/gamepacket"
	"net"
)

// Module enums
type ModuleEnum int

const (
	ConnectionModuleEnum ModuleEnum = iota
	LobbyModuleEnum
	MovementModuleEnum
	CollisionModuleEnum
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
}

var modules []ServerModule

func init() {
	movementModule := NewMovementModule()

	modules = []ServerModule{
		NewConnectionModule(),
		NewLobbyModule(),
		movementModule,
	}
	movementModule.StartWorkers()

	// Print module status after initialization
	fmt.Println("\n🚀 Module initialization complete:")
	ListModules()
	fmt.Println()
}

func dispatchPacket(conn net.PacketConn, addr net.Addr, pkt *gamepacket.GamePacket) {
	for _, m := range modules {
		if m.CanHandle(pkt) {
			m.Handle(conn, addr, pkt)
			return
		}
	}
	fmt.Println("⚠️ No module handled packet")
}

func shutdownModules() {
	fmt.Println("🛑 Shutting down all modules...")

	for _, module := range modules {
		if movementModule, ok := module.(*MovementModule); ok {
			fmt.Println("🛑 Stopping movement workers...")
			movementModule.StopWorkers()
		}
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
