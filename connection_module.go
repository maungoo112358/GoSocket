package main

import (
	"fmt"
	"gosocket/gamepacket"
	"net"

	"google.golang.org/protobuf/proto"
)

type ConnectionModule struct{}

func NewConnectionModule() *ConnectionModule {
	return &ConnectionModule{}
}

func (m *ConnectionModule) CanHandle(pkt *gamepacket.GamePacket) bool {
	return pkt.GetHandshakeRequest() != nil || pkt.GetHeartbeat() != nil
}

func (m *ConnectionModule) Handle(conn net.PacketConn, addr net.Addr, pkt *gamepacket.GamePacket) {
	switch {
	case pkt.GetHandshakeRequest() != nil:
		handleHandshake(conn, addr, pkt)
	case pkt.GetHeartbeat() != nil:
		handleHeartbeat(conn, addr, pkt)
	}
}

func handleHandshake(conn net.PacketConn, addr net.Addr, pkt *gamepacket.GamePacket) {
	req := pkt.GetHandshakeRequest()
	if req == nil {
		return
	}

	privateID := generatePrivateID()
	publicID := generatePublicID()
	addClient(privateID, publicID, req.ClientName, addr)

	response := &gamepacket.GamePacket{
		Seq: pkt.Seq,
		HandshakeResponse: &gamepacket.HandshakeResponse{
			PrivateId: privateID,
			PublicId:  publicID,
		},
	}

	if data, err := proto.Marshal(response); err == nil {
		conn.WriteTo(data, addr)
	}
}

func handleHeartbeat(conn net.PacketConn, addr net.Addr, pkt *gamepacket.GamePacket) {
	hb := pkt.GetHeartbeat()
	if hb == nil {
		return
	}

	if !updateHeartbeat(hb.ClientId) {
		fmt.Printf("⚠️ Invalid heartbeat from unknown client: %s\n", hb.ClientId)
		return
	}

	ack := &gamepacket.GamePacket{
		Seq:          pkt.Seq,
		HeartbeatAck: &gamepacket.HeartbeatAck{ClientId: hb.ClientId},
	}

	if data, err := proto.Marshal(ack); err == nil {
		conn.WriteTo(data, addr)
	}
}
