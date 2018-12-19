package credsyncer

import (
	"time"

	rfv1beta3 "github.com/refunc/refunc/pkg/apis/refunc/v1beta3"
	"github.com/refunc/refunc/pkg/env"
	"github.com/refunc/refunc/pkg/utils/rfutil"
)

// Provider is interface for a vault to issue credentials
type Provider interface {
	IssueKeyPair(fni *rfv1beta3.Funcinst) (accessKey, secretKey string, err error)
	IssueAccessToken(fni *rfv1beta3.Funcinst) (accessToken string, err error)
}

// NewSimpleProvider creates a creds provider simply forwarding keys and token in current env
func NewSimpleProvider() Provider {
	return new(simpleProvider)
}

// NewGeneratedProvider creates a creds provider generate random keys, issues token using private key in current env
func NewGeneratedProvider(lifetime time.Duration) Provider {
	return &generateProvider{
		lifetime: lifetime,
	}
}

type simpleProvider struct {
}

func (sp *simpleProvider) IssueKeyPair(fni *rfv1beta3.Funcinst) (accessKey, secretKey string, err error) {
	return env.GlobalAccessKey, env.GlobalSecretKey, nil
}

func (sp *simpleProvider) IssueAccessToken(fni *rfv1beta3.Funcinst) (accessToken string, err error) {
	return env.GlobalToken, nil
}

type generateProvider struct {
	lifetime time.Duration
}

func (sp *generateProvider) IssueKeyPair(fni *rfv1beta3.Funcinst) (accessKey, secretKey string, err error) {
	accessKey, secretKey = env.RandomCredentials()
	return
}

func (sp *generateProvider) IssueAccessToken(fni *rfv1beta3.Funcinst) (accessToken string, err error) {
	return rfutil.IssueToken(fni.Namespace+"/"+fni.Name, fni.Spec.Runtime.Permissions, sp.lifetime)
}
