package server

import (
	"context"
	"fmt"
	"gosocket/gamepacket"
	"gosocket/internal/client"
	"gosocket/internal/registry"
	"net"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"google.golang.org/protobuf/proto"
)

const (
	MaxWorkers    = 10
	PacketBuffer  = 100
	ReadTimeout   = 100 * time.Millisecond
	MaxPacketSize = 1024
)

type Server struct {
	conn    net.PacketConn
	ctx     context.Context
	cancel  context.CancelFunc
	workers sync.WaitGroup
}

type PacketWork struct {
	Data []byte
	Addr net.Addr
}

func StartServer(port string) error {
	server, err := NewServer(port)
	if err != nil {
		return fmt.Errorf("failed to create server: %w", err)
	}
	defer server.Close()

	fmt.Printf("✅ UDP server listening on %s\n", port)

	server.StartHeartbeatChecker()
	server.SetupGracefulShutdown()

	return server.Run()
}

func NewServer(port string) (*Server, error) {
	conn, err := net.ListenPacket("udp", port)
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithCancel(context.Background())

	return &Server{
		conn:   conn,
		ctx:    ctx,
		cancel: cancel,
	}, nil
}

func (s *Server) Run() error {
	packetChan := make(chan PacketWork, PacketBuffer)

	// Start worker pool
	s.startWorkers(packetChan)

	// Main packet reading loop
	return s.readPackets(packetChan)
}

func (s *Server) startWorkers(packetChan <-chan PacketWork) {
	for i := 0; i < MaxWorkers; i++ {
		s.workers.Add(1)
		go s.packetWorker(packetChan)
	}
}

func (s *Server) readPackets(packetChan chan<- PacketWork) error {
	defer close(packetChan)

	buffer := make([]byte, MaxPacketSize)

	for {
		select {
		case <-s.ctx.Done():
			s.workers.Wait()
			fmt.Println("🛑 Server shutdown complete")
			return nil
		default:
		}

		// Read with timeout to check context periodically
		s.conn.SetReadDeadline(time.Now().Add(ReadTimeout))
		n, addr, err := s.conn.ReadFrom(buffer)

		if err != nil {
			if s.isTimeoutError(err) {
				continue // Check context and retry
			}
			fmt.Printf("❌ Read error: %v\n", err)
			continue
		}

		// Send packet to worker pool
		packet := PacketWork{
			Data: append([]byte(nil), buffer[:n]...), // Copy data
			Addr: addr,
		}

		select {
		case packetChan <- packet:
		case <-s.ctx.Done():
			return nil
		default:
			fmt.Printf("⚠️ Packet dropped - worker pool busy\n")
		}
	}
}

func (s *Server) packetWorker(packets <-chan PacketWork) {
	defer s.workers.Done()

	for {
		select {
		case work, ok := <-packets:
			if !ok {
				return // Channel closed
			}
			s.processPacket(work)
		case <-s.ctx.Done():
			return
		}
	}
}

func (s *Server) processPacket(work PacketWork) {
	defer func() {
		if r := recover(); r != nil {
			fmt.Printf("🔥 Panic recovered from %s: %v\n", work.Addr.String(), r)
		}
	}()

	var packet gamepacket.GamePacket
	if err := proto.Unmarshal(work.Data, &packet); err != nil {
		fmt.Printf("❌ Unmarshal error from %s: %v\n", work.Addr, err)
		return
	}

	// Route packet to appropriate module
	registry.DispatchPacket(s.conn, work.Addr, &packet)
}

func (s *Server) SetupGracefulShutdown() {
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, os.Interrupt, syscall.SIGTERM, syscall.SIGINT, syscall.SIGHUP)

	go func() {
		<-signals
		fmt.Println("🛑 Shutdown signal received...")
		s.Shutdown()

		registry.ShutdownModules()

		s.cancel()
	}()
}

func (s *Server) Shutdown() {
	clients := client.GetAllClients()
	if len(clients) == 0 {
		return
	}

	// Create shutdown message
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

	// Send to all clients concurrently
	var wg sync.WaitGroup
	for _, c := range clients {
		wg.Add(1)
		go func(cl *client.ClientInfo) {
			defer wg.Done()
			s.conn.WriteTo(data, cl.Address)
		}(c)
	}

	wg.Wait()
	fmt.Printf("📤 Shutdown message sent to %d clients\n", len(clients))
}

func (s *Server) StartHeartbeatChecker() {
	go client.StartHeartbeatChecker(s.conn)
}

func (s *Server) Close() {
	s.cancel()

	registry.ShutdownModules()

	if s.conn != nil {
		s.conn.Close()
	}
}

func (s *Server) isTimeoutError(err error) bool {
	if netErr, ok := err.(net.Error); ok {
		return netErr.Timeout()
	}
	return false
}
