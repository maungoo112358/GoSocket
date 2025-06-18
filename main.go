package main

import (
	"fmt"
	"gosocket/gamepacket"
	"net"
	"os"
	"os/signal"
	"syscall"

	"google.golang.org/protobuf/proto"
)

func main() {
	addr := ":9999"
	conn, err := net.ListenPacket("udp", addr)
	if err != nil {
		panic(err)
	}
	defer conn.Close()

	fmt.Println("✅ UDP server listening on: ", addr)
	go startHeartbeatChecker()

	signals := make(chan os.Signal, 1)
	signal.Notify(signals, os.Interrupt, syscall.SIGTERM, syscall.SIGINT, syscall.SIGHUP)

	go func() {
		<-signals
		notifyClientsBeforeShutdown(conn)
		fmt.Println("🛑 Server is shutting down...")
		os.Exit(0)
	}()

	buf := make([]byte, 1024)
	for {

		n, remote, err := conn.ReadFrom(buf)

		if err != nil {
			fmt.Println("❌ read error: ", err)
			continue
		}

		go func(data []byte, addr net.Addr) {
			defer func() {
				if r := recover(); r != nil {
					fmt.Printf("🔥 Panic recovered from %s: %v\n", addr.String(), r)
				}
			}()

			var pkt gamepacket.GamePacket
			err := proto.Unmarshal(data[:n], &pkt)
			if err != nil {
				fmt.Println("❌ unmarshal error: ", err)
				return
			}
			dispatchPacket(conn, addr, &pkt)

		}(append([]byte{}, buf[:n]...), remote)

	}
}
