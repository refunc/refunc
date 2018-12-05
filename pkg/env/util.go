package env

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
)

const (
	// Maximum length for Minio access key.
	// There is no max length enforcement for access keys
	accessKeyMaxLen = 20

	// Maximum secret key length for Minio, this
	// is used when autogenerating new credentials.
	// There is no max length enforcement for secret keys
	secretKeyMaxLen = 40

	// Alpha numeric table used for generating access keys.
	alphaNumericTable = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZ"

	// Total length of the alpha numeric table.
	alphaNumericTableLen = byte(len(alphaNumericTable))
)

// RandomCredentials returns valid key and secret
func RandomCredentials() (key, secret string) {
	readBytes := func(size int) (data []byte) {
		data = make([]byte, size)
		if n, err := rand.Read(data); err != nil {
			panic(err)
		} else if n != size {
			panic(fmt.Errorf("not enough data read. expected: %v, got: %v", size, n))
		}
		return
	}

	// Generate access key.
	keyBytes := readBytes(accessKeyMaxLen)
	for i := 0; i < accessKeyMaxLen; i++ {
		keyBytes[i] = alphaNumericTable[keyBytes[i]%alphaNumericTableLen]
	}
	key = string(keyBytes)

	// Generate secret key.
	keyBytes = readBytes(secretKeyMaxLen)
	secret = string([]byte(base64.URLEncoding.EncodeToString(keyBytes))[:secretKeyMaxLen])

	return
}
