package main

import (
	"fmt"
	"strings"
	"sync"

	crand "crypto/rand"
)

var (
	privateIDStore = make(map[string]struct{})
	publicIDStore  = make(map[string]struct{})
	idMutex        sync.Mutex
)

var (
	publicIDCounter int
	publicIDMutex   sync.Mutex
)

func generateSecureDigits(n int) string {
	b := make([]byte, n)
	crand.Read(b)
	for i := range b {
		b[i] = '0' + (b[i] % 10)
	}
	return string(b)
}

func generateUniqueID(prefix string, store map[string]struct{}) string {
	for {
		id := fmt.Sprintf("%s%s", prefix, generateSecureDigits(6))
		idlower := strings.ToLower(id)

		idMutex.Lock()
		_, exists := store[idlower]

		if !exists {
			store[idlower] = struct{}{}
			idMutex.Unlock()
			return id
		}
		idMutex.Unlock()
	}
}

func generatePublicID() string {
	publicIDMutex.Lock()
	publicIDCounter++
	id := fmt.Sprintf("Client_%d", publicIDCounter)
	publicIDStore[strings.ToLower(id)] = struct{}{}
	publicIDMutex.Unlock()
	return id
}

func generatePrivateID() string {
	return generateUniqueID("Client_", privateIDStore)
}

// Clean up client IDs from maps
func cleanupClientIDs(privateID, publicID string) {
	idMutex.Lock()
	defer idMutex.Unlock()

	delete(privateIDStore, strings.ToLower(privateID))
	delete(publicIDStore, strings.ToLower(publicID))
}
