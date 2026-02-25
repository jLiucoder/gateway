//hash key functions, right now only support environemtn variable, will support postgres aki key store

package main

import (
	"crypto/sha256"
	"encoding/hex"
)

func hashKey(key string) string {
	hash := sha256.Sum256([]byte(key))
	return hex.EncodeToString(hash[:])
}
