package verifier

import (
	"crypto/ecdsa"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"strings"
	"sync"

	k8sinformers "k8s.io/client-go/informers"

	jwt "github.com/golang-jwt/jwt"
	"github.com/refunc/refunc/pkg/builtins"
	"github.com/refunc/refunc/pkg/credsyncer"
	"github.com/refunc/refunc/pkg/env"
	rfinformers "github.com/refunc/refunc/pkg/generated/informers/externalversions"
	rflistersv1 "github.com/refunc/refunc/pkg/generated/listers/refunc/v1beta3"
	"github.com/refunc/refunc/pkg/messages"
	"github.com/refunc/refunc/pkg/utils/k8sutil"
)

// RegisterVerifer registers verfiy@builtins
func RegisterVerifer(
	stopC <-chan struct{},
	namespace string,
	refuncInformers rfinformers.SharedInformerFactory,
	kubeInformers k8sinformers.SharedInformerFactory,
) (err error) {
	initOnce.Do(func() {
		s := &credsStore{
			fniLister: refuncInformers.Refunc().V1beta3().Funcinsts().Lister(),
		}
		var syncer credsyncer.Syncer
		syncer, err = credsyncer.NewCredSyncer(namespace, env.GlobalBucket, s, refuncInformers, kubeInformers)
		if err != nil {
			return
		}
		store = s
		startC := make(chan struct{})
		go func() {
			close(startC)
			syncer.Run(stopC)
		}()
		<-startC
		builtins.RegisterBuiltin("verify", "", verify)
	})
	return nil
}

// indexed by accessKey
type credsStore struct {
	sync.Map

	fniLister rflistersv1.FuncinstLister
}

var (
	initOnce sync.Once
	store    *credsStore
)

func (r *credsStore) AddCreds(creds *credsyncer.FlatCreds) error {
	if creds.FuncinstID != "" {
		r.Store(creds.AccessKey, creds.FuncinstID)
	} else {
		r.Store(creds.AccessKey, creds)
	}
	return nil

}

func (r *credsStore) DeleteCreds(accessKey string) error {
	r.Delete(accessKey)
	return nil
}

// errors
var (
	errInvalidRequest    = errors.New("verify: invalid requests, either token or (accesskey, secretkey) must be provided")
	errAccessKeyNotFound = errors.New("verify: the accessKey your provided is not in our records")
	errInvalidSecretKey  = errors.New("verify: the secretKey your provided invalid")
	errInvalidCreds      = errors.New("verify: invalid credentials")
)

const tokenPrefix = "eyJhbGciOiJFUzI1NiIsInR5cCI6IkpXVCJ9."

func verify(request *messages.InvokeRequest) (token interface{}, err error) {
	var args struct {
		Token string `json:"token,omitempty"`
		// access&secret
		AccessKey string `json:"accessKey,omitempty"`
		SecretKey string `json:"secretKey,omitempty"`
	}
	if err = json.Unmarshal(request.Args, &args); err != nil {
		return
	}
	// prefer to verify token first
	if args.Token != "" {
		// handle short token
		tokenStr := args.Token
		if !strings.HasPrefix(tokenStr, tokenPrefix) {
			// try to prepend
			tokenStr = tokenPrefix + tokenStr
		}
		token, err := jwt.ParseWithClaims(tokenStr, jwt.MapClaims{}, func(t *jwt.Token) (interface{}, error) {
			if _, ok := t.Method.(*jwt.SigningMethodECDSA); !ok {
				return nil, fmt.Errorf("expect token signed with ECDSA but got %v", t.Header["alg"])
			}
			return publicKey()
		})
		if err != nil {
			return nil, err
		}
		return token.Claims, nil
	}

	if args.AccessKey != "" && args.SecretKey != "" {
		val, has := store.Load(args.AccessKey)
		if !has {
			return nil, errAccessKeyNotFound
		}
		switch creds := val.(type) {
		case string:
			// funcinst
			splitter := strings.SplitN(creds, "/", 2)
			fni, err := store.fniLister.Funcinsts(splitter[0]).Get(splitter[1])
			if err != nil {
				if k8sutil.IsResourceNotFoundError(err) {
					return nil, errInvalidCreds
				}
				return nil, err
			}
			if fni.Spec.Runtime.Credentials.AccessKey == args.AccessKey && fni.Spec.Runtime.Credentials.SecretKey == args.SecretKey {
				return credsyncer.NewCreds(fni, env.GlobalBucket), nil
			}
			return nil, errInvalidCreds
		case *credsyncer.FlatCreds:
			if creds.AccessKey == args.AccessKey && creds.SecretKey == args.SecretKey {
				return creds, nil
			}
			return nil, errInvalidSecretKey
		}
	}
	return nil, errInvalidRequest
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
}
