package main

import (
	"gosocket/internal/server"
)

func main() {
	server.StartServer(":9999")
}
