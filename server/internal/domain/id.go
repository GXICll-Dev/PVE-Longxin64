package domain

import (
	"crypto/rand"
	"fmt"
)

// NewID returns a RFC 4122 version 4 UUID without requiring callers to depend
// on a storage implementation or an external UUID package.
func NewID() string {
	var value [16]byte
	if _, err := rand.Read(value[:]); err != nil {
		panic(fmt.Sprintf("generate UUID: %v", err))
	}
	value[6] = (value[6] & 0x0f) | 0x40
	value[8] = (value[8] & 0x3f) | 0x80
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x",
		value[0:4], value[4:6], value[6:8], value[8:10], value[10:16])
}
