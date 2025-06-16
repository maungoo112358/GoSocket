package main

import (
	"fmt"
	"gosocket/gamepacket"
	"net"

	"google.golang.org/protobuf/proto"
)

func main() {
	addr := ":9999"
	conn, err := net.ListenPacket("udp", addr)
	if err != nil {
		panic(err)
	}
	defer conn.Close()

	fmt.Println("UDP server listening on: ", addr)

	buf := make([]byte, 1024)
	for {

		n, remote, err := conn.ReadFrom(buf)

		if err != nil {
			fmt.Println("read error: ", err)
			continue
		}

		var pkt gamepacket.GamePacket

		err = proto.Unmarshal(buf[:n], &pkt)

		if err != nil {
			fmt.Println("unmarshal error: ", err)
			continue
		}

		fmt.Printf("From %v → Seq: %d\n", remote, pkt.Seq)
	}

}
