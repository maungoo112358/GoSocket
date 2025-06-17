package main

import (
	"net"
	"sync"
	"time"
)

type Client struct {
	PublicID  string
	PrivateID string
	Addr      net.Addr
	LastSeen  time.Time
}

var (
	clients   = make(map[string]*Client)
	clientsMu sync.Mutex
)

func updateHeartbeat(privateID string) {
	clientsMu.Lock()
	defer clientsMu.Unlock()

	if client, ok := clients[privateID]; ok {
		client.LastSeen = time.Now()
	}
}

func heartbeatWatcher() {
	ticker := time.NewTicker(5 * time.Second)
	for range ticker.C {
		var expiredIDs []string

		clientsMu.Lock()
		now := time.Now()
		for id, client := range clients {
			if now.Sub(client.LastSeen) > 5*time.Second {
				expiredIDs = append(expiredIDs, id)
			}
		}
		clientsMu.Unlock()

		for _, id := range expiredIDs {
			handleDisconnection(id)
		}
	}
}
