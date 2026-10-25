package telegram

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
)

// chunkObjectName returns an opaque name for one stored chunk. Grouping and
// order live only in Nimbus metadata; the Telegram document carries neither a
// file identity nor a part number.
func chunkObjectName() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", fmt.Errorf("telegram: random chunk name: %w", err)
	}
	return hex.EncodeToString(b[:]) + ".bin", nil
}
