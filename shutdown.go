package main

import (
	"gosocket/gamepacket"
	"net"

	"google.golang.org/protobuf/proto"
)

func notifyClientsBeforeShutdown(conn net.PacketConn) {
	allClientsMu.Lock()
	defer allClientsMu.Unlock()

	for _, client := range allClients {
		response := &gamepacket.GamePacket{
			Seq: 999,
			ServerStatus: &gamepacket.ServerStatus{
				Message: "Server is shutting down",
			},
		}

		data, _ := proto.Marshal(response)
		conn.WriteTo(data, client.Address)
	}
}
