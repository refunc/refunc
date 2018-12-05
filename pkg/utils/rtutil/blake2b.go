package rtutil

import (
	"encoding/base64"
	"strings"

	"golang.org/x/crypto/blake2b"
)

// Defaults for passwords
const (
	// Support for blake2b stored passwords and tokens.
	blake2bPrefix = "$b2$"
	alphabet      = "./ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789"
)

var bcEncoding = base64.NewEncoding(alphabet)

// IsBlake2b checks whether the given password or token is bcrypted.
func IsBlake2b(password string) bool {
	return strings.HasPrefix(password, blake2bPrefix)
}

// EncryptKey encode given key using blake2b
func EncryptKey(key string) string {
	cb := blake2b.Sum256([]byte(key))
	return blake2bPrefix + bcEncoding.EncodeToString(cb[:])[:23]
}

// ComparePassword compares local and reomote password
func ComparePassword(local, remote string) bool {
	// Check to see if the server password is a bcrypt hash
	if IsBlake2b(local) {
		return local == EncryptKey(remote)
	} else if local != remote {
		return false
	}
	return true
}
