package main

import (
	crand "crypto/rand"
	"fmt"
	"math/rand"
	"strings"
	"sync"
	"time"
)

// ID generation configuration
const (
	PrivateIDPrefix = "Client_"
	TempIDPrefix    = "temp_"
	SessionPrefix   = "sess_"
	IDDigitLength   = 6
)

// ID storage to prevent duplicates
var (
	usedPrivateIDs = make(map[string]struct{})
	usedPublicIDs  = make(map[string]struct{})
	idStoreMutex   sync.Mutex
)

// === Public ID Generation ===

// GeneratePrivateID creates a unique private client identifier
func GeneratePrivateID() string {
	return generateUniqueID(PrivateIDPrefix, usedPrivateIDs)
}

// GenerateTempID creates a temporary ID for pending clients
func GenerateTempID() string {
	timestamp := time.Now().UnixNano()
	randomNum := rand.Intn(10000)
	return fmt.Sprintf("%s%d_%d", TempIDPrefix, timestamp, randomNum)
}

// GenerateSessionToken creates a session token for client authentication
func GenerateSessionToken() string {
	timestamp := time.Now().UnixNano()
	randomNum := rand.Intn(10000)
	return fmt.Sprintf("%s%d_%d", SessionPrefix, timestamp, randomNum)
}

// === ID Management ===

// ReservePublicID marks a public ID as used (for username validation)
func ReservePublicID(publicID string) bool {
	idStoreMutex.Lock()
	defer idStoreMutex.Unlock()

	idLower := strings.ToLower(publicID)

	if _, exists := usedPublicIDs[idLower]; exists {
		return false // Already taken
	}

	usedPublicIDs[idLower] = struct{}{}
	return true
}

// ReleasePublicID frees up a public ID for reuse
func ReleasePublicID(publicID string) {
	idStoreMutex.Lock()
	defer idStoreMutex.Unlock()

	idLower := strings.ToLower(publicID)
	delete(usedPublicIDs, idLower)
}

// ReleasePrivateID frees up a private ID for reuse
func ReleasePrivateID(privateID string) {
	idStoreMutex.Lock()
	defer idStoreMutex.Unlock()

	idLower := strings.ToLower(privateID)
	delete(usedPrivateIDs, idLower)
}

// IsPublicIDAvailable checks if a public ID can be used
func IsPublicIDAvailable(publicID string) bool {
	idStoreMutex.Lock()
	defer idStoreMutex.Unlock()

	idLower := strings.ToLower(publicID)
	_, exists := usedPublicIDs[idLower]
	return !exists
}

// === Validation Functions ===

// IsValidUsername checks if a username meets requirements
func IsValidUsername(username string) (bool, string) {
	trimmed := strings.TrimSpace(username)

	if len(trimmed) < 3 {
		return false, "Username must be at least 3 characters"
	}

	if len(trimmed) > 20 {
		return false, "Username must be 20 characters or less"
	}

	// Check for invalid characters (optional - add as needed)
	if strings.ContainsAny(trimmed, " \t\n\r") {
		return false, "Username cannot contain spaces or special whitespace"
	}

	return true, ""
}

// === Internal Helper Functions ===

// generateUniqueID creates a unique ID with the given prefix and stores it
func generateUniqueID(prefix string, store map[string]struct{}) string {
	maxAttempts := 1000 // Prevent infinite loops

	for attempt := 0; attempt < maxAttempts; attempt++ {
		digits := generateSecureDigits(IDDigitLength)
		id := fmt.Sprintf("%s%s", prefix, digits)
		idLower := strings.ToLower(id)

		idStoreMutex.Lock()
		_, exists := store[idLower]

		if !exists {
			store[idLower] = struct{}{}
			idStoreMutex.Unlock()
			return id
		}
		idStoreMutex.Unlock()
	}

	// Fallback if we somehow can't generate unique ID
	panic(fmt.Sprintf("Failed to generate unique ID with prefix %s after %d attempts", prefix, maxAttempts))
}

// generateSecureDigits creates cryptographically secure random digits
func generateSecureDigits(length int) string {
	if length <= 0 {
		return ""
	}

	bytes := make([]byte, length)
	_, err := crand.Read(bytes)
	if err != nil {
		// Fallback to math/rand if crypto/rand fails
		return generateInsecureDigits(length)
	}

	// Convert bytes to digits (0-9)
	for i := range bytes {
		bytes[i] = '0' + (bytes[i] % 10)
	}

	return string(bytes)
}

// generateInsecureDigits fallback using math/rand
func generateInsecureDigits(length int) string {
	digits := make([]byte, length)
	for i := range digits {
		digits[i] = '0' + byte(rand.Intn(10))
	}
	return string(digits)
}

// === Statistics and Debugging ===

// GetIDStats returns statistics about ID usage
func GetIDStats() (privateCount, publicCount int) {
	idStoreMutex.Lock()
	defer idStoreMutex.Unlock()

	return len(usedPrivateIDs), len(usedPublicIDs)
}

// CleanupUnusedIDs removes IDs that are no longer associated with active clients
// This should be called periodically to prevent memory leaks
func CleanupUnusedIDs() {
	activeClients := GetAllClients()

	// Build sets of currently used IDs
	activePrivateIDs := make(map[string]struct{})
	activePublicIDs := make(map[string]struct{})

	for _, client := range activeClients {
		activePrivateIDs[strings.ToLower(client.PrivateID)] = struct{}{}
		activePublicIDs[strings.ToLower(client.PublicID)] = struct{}{}
	}

	idStoreMutex.Lock()
	defer idStoreMutex.Unlock()

	// Count what we're cleaning up
	privateCleanedCount := 0
	publicCleanedCount := 0

	// Clean up unused private IDs
	for privateID := range usedPrivateIDs {
		if _, isActive := activePrivateIDs[privateID]; !isActive {
			delete(usedPrivateIDs, privateID)
			privateCleanedCount++
		}
	}

	// Clean up unused public IDs
	for publicID := range usedPublicIDs {
		if _, isActive := activePublicIDs[publicID]; !isActive {
			delete(usedPublicIDs, publicID)
			publicCleanedCount++
		}
	}

	if privateCleanedCount > 0 || publicCleanedCount > 0 {
		fmt.Printf("🧹 Cleaned up %d private IDs, %d public IDs\n",
			privateCleanedCount, publicCleanedCount)
	}
}

// === Initialization ===

func init() {
	// Seed the math/rand package for fallback cases
	rand.Seed(time.Now().UnixNano())
}
