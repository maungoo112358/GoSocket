package main

import (
	"context"
	"fmt"
	"gosocket/gamepacket"
	"net"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"google.golang.org/protobuf/proto"
)

var modules []ServerModule

func init() {
	modules = []ServerModule{
		NewConnectionModule(),
		NewLobbyModule(),
		NewMovementModule(),
	}
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

func startServer(port string) {
	conn, err := net.ListenPacket("udp", port)
	if err != nil {
		panic(err)
	}
	defer conn.Close()

	fmt.Println("✅ UDP server listening on: ", port)

	// Create context for graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go startHeartbeatChecker(conn)

	// Signal handling
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, os.Interrupt, syscall.SIGTERM, syscall.SIGINT, syscall.SIGHUP)

	go func() {
		<-signals
		fmt.Println("🛑 Shutdown signal received...")
		shutdownServer(conn)
		cancel() // Cancel context to stop main loop
	}()

	// Worker pool for packet processing
	const numWorkers = 10
	packetChan := make(chan packetWork, 100)

	var wg sync.WaitGroup
	for i := 0; i < numWorkers; i++ {
		wg.Add(1)
		go packetWorker(ctx, conn, packetChan, &wg)
	}

	// Main read loop
	buf := make([]byte, 1024)
	for {
		select {
		case <-ctx.Done():
			close(packetChan)
			wg.Wait()
			fmt.Println("🛑 Server shutdown complete")
			return
		default:
		}

		// Set read timeout to check context periodically
		conn.SetReadDeadline(time.Now().Add(100 * time.Millisecond))
		n, remote, err := conn.ReadFrom(buf)

		if err != nil {
			if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
				continue // Check context and retry
			}
			fmt.Printf("❌ read error: %v\n", err)
			continue
		}

		// Copy data and send to worker pool
		data := make([]byte, n)
		copy(data, buf[:n])

		select {
		case packetChan <- packetWork{data: data, addr: remote}:
		case <-ctx.Done():
			return
		default:
			// Channel full - drop packet
			fmt.Printf("⚠️ Packet dropped - worker pool busy\n")
		}
	}
}

type packetWork struct {
	data []byte
	addr net.Addr
}

func packetWorker(ctx context.Context, conn net.PacketConn, packets <-chan packetWork, wg *sync.WaitGroup) {
	defer wg.Done()

	for {
		select {
		case work, ok := <-packets:
			if !ok {
				return // Channel closed
			}
			processPacket(conn, work.data, work.addr)
		case <-ctx.Done():
			return
		}
	}
}

func processPacket(conn net.PacketConn, data []byte, addr net.Addr) {
	defer func() {
		if r := recover(); r != nil {
			fmt.Printf("🔥 Panic recovered from %s: %v\n", addr.String(), r)
		}
	}()

	var pkt gamepacket.GamePacket
	if err := proto.Unmarshal(data, &pkt); err != nil {
		fmt.Printf("❌ unmarshal error from %s: %v\n", addr, err)
		return
	}

	dispatchPacket(conn, addr, &pkt)
}

func shutdownServer(conn net.PacketConn) {
	allClientsMu.RLock()

	// Collect all clients
	var clients []*ClientInfo
	for _, client := range allClients {
		clients = append(clients, client)
	}
	allClientsMu.RUnlock()

	if len(clients) == 0 {
		return
	}

	// Marshal once
	response := &gamepacket.GamePacket{
		Seq: 999,
		ServerStatus: &gamepacket.ServerStatus{
			Message: "Server is shutting down",
		},
	}
	data, err := proto.Marshal(response)
	if err != nil {
		fmt.Printf("❌ Failed to marshal shutdown message: %v\n", err)
		return
	}

	// Send concurrently
	var wg sync.WaitGroup
	for _, client := range clients {
		wg.Add(1)
		go func(c *ClientInfo) {
			defer wg.Done()
			conn.WriteTo(data, c.Address)
		}(client)
	}

	wg.Wait()
	fmt.Printf("📤 Shutdown message sent to %d clients\n", len(clients))
}
