package utils

import (
	"fmt"
	"gosocket/gamepacket"
	"math/rand"
	"net"

	"google.golang.org/protobuf/proto"
)

// SendToAddress sends a packet to a specific network address
func SendToAddress(conn net.PacketConn, packet *gamepacket.GamePacket, addr net.Addr) error {
	data, err := proto.Marshal(packet)
	if err != nil {
		return fmt.Errorf("failed to marshal packet: %w", err)
	}

	_, err = conn.WriteTo(data, addr)
	if err != nil {
		return fmt.Errorf("failed to send to %s: %w", addr.String(), err)
	}

	return nil
}

// GenerateRandomSeq creates a random sequence number for packets
func GenerateRandomSeq() uint32 {
	return uint32(rand.Intn(100000))
}