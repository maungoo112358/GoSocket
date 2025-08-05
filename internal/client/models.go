package client

import (
	"net"
	"time"
)

type ClientInfo struct {
	PrivateID     string
	PublicID      string
	Name          string
	Address       net.Addr
	LastHeartbeat time.Time
	ConnectedAt   time.Time
	ColorHex      string
	SessionToken  string
	InLobby       bool
}

type PendingClient struct {
	TempID    string
	Address   net.Addr
	CreatedAt time.Time
}

type LobbyPosition struct {
	X, Y, Z float32
}

func (c *ClientInfo) WasInLobby() bool {
	return c.InLobby && c.ColorHex != ""
}
