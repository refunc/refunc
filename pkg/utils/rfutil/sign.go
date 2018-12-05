package rfutil

import (
	"crypto/ecdsa"
	"errors"
	"io/ioutil"
	"os"
	"sync"

	jwt "github.com/dgrijalva/jwt-go"
)

// well known errros
var (
	ErrMissingPrivateKeyFile = errors.New(`creds: "ECDSA_KEY_FILE" not specified`)

	// private ECDSA key to sign token
	privateECDSAKeyFile string
)

// Sign signs given claims with ES256
func Sign(claims jwt.Claims) (string, error) {
	key, err := getPrivateKey()
	if err != nil {
		return "", err
	}
	// create a new token
	token := jwt.NewWithClaims(signAlg, claims)
	return token.SignedString(key)
}

var (
	// TODO: make sign alg configurable
	signAlg     = jwt.GetSigningMethod("ES256")
	ecdsaKey    *ecdsa.PrivateKey
	loadKeyErr  error
	loadKeyOnce sync.Once
)

func getPrivateKey() (*ecdsa.PrivateKey, error) {
	loadKeyOnce.Do(func() {
		if privateECDSAKeyFile == "" {
			loadKeyErr = ErrMissingPrivateKeyFile
			return
		}
		f, err := os.Open(privateECDSAKeyFile)
		if err != nil {
			loadKeyErr = err
			return
		}
		defer f.Close()
		keyBytes, err := ioutil.ReadAll(f)
		if err != nil {
			loadKeyErr = err
			return
		}
		ecdsaKey, loadKeyErr = jwt.ParseECPrivateKeyFromPEM(keyBytes)
	})
	return ecdsaKey, loadKeyErr
}

func init() {
	privateECDSAKeyFile = os.Getenv("ECDSA_PRIVATEKEY_FILE")
}
