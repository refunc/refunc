package sign

import (
	"crypto/ecdsa"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"strings"
	"sync"

	jwt "github.com/dgrijalva/jwt-go"
	"github.com/refunc/refunc/pkg/builtins"
	"github.com/refunc/refunc/pkg/messages"
	"github.com/refunc/refunc/pkg/utils/rfutil"
)

var (
	errInvalidRequest = errors.New("sign: token must be provided")
	errUnauthorized   = errors.New("sign: do not have rights to issue a token")
)

func signToken(request *messages.InvokeRequest) (token interface{}, err error) {
	var args struct {
		Token  string        `json:"token"`
		Claims jwt.MapClaims `json:"claims,omitempty"`
	}
	if err = json.Unmarshal(request.Args, &args); err != nil {
		return
	}
	if args.Token == "" {
		err = errInvalidRequest
		return
	}
	if err = verify(args.Token); err != nil {
		return
	}
	tokenStr, err := rfutil.Sign(args.Claims)
	if err != nil {
		return
	}
	// reuse args' structure
	args.Token = tokenStr
	args.Claims = nil
	return args, nil
}

func verify(tokenString string) error {
	token, err := jwt.ParseWithClaims(tokenString, jwt.MapClaims{}, func(t *jwt.Token) (interface{}, error) {
		if _, ok := t.Method.(*jwt.SigningMethodECDSA); !ok {
			return nil, fmt.Errorf("expect token signed with ECDSA but got %v", t.Header["alg"])
		}
		return publicKey()
	})

	if err != nil {
		// break on first correctly validated token
		return err
	}
	v, has := token.Claims.(jwt.MapClaims)["sign"]
	if !has {
		return errUnauthorized
	}
	switch canSign := v.(type) {
	case bool:
		if canSign {
			return nil
		}
	case string:
		if strings.ToLower(canSign) == "true" {
			return nil
		}
	}
	return errUnauthorized
}

var (
	// public ECDSA key to sign token
	publicECDSAKeyFile string

	pubKey      *ecdsa.PublicKey
	loadKeyErr  error
	loadKeyOnce sync.Once
)

func publicKey() (*ecdsa.PublicKey, error) {
	loadKeyOnce.Do(func() {
		if publicECDSAKeyFile == "" {
			loadKeyErr = errors.New("sign: missing public to verify")
			return
		}
		f, err := os.Open(publicECDSAKeyFile)
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
		pubKey, loadKeyErr = jwt.ParseECPublicKeyFromPEM(keyBytes)
	})
	return pubKey, loadKeyErr
}

func init() {
	publicECDSAKeyFile = os.Getenv("ECDSA_PUBLICKEY_FILE")
	builtins.RegisterBuiltin("sign", "", signToken)
}
