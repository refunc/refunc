package rfutil

import (
	"errors"
	"time"

	jwt "github.com/golang-jwt/jwt"
	rfv1beta3 "github.com/refunc/refunc/pkg/apis/refunc/v1beta3"
)

// IssueToken issues a token to connect to nats for a funcinst
func IssueToken(sub string, perms rfv1beta3.Permissions, exp time.Duration) (string, error) {
	claims := funcDefTokenModel{
		Subject:     sub,
		Expires:     jwt.TimeFunc().Add(exp).Unix(),
		Permissions: perms,
	}
	return Sign(&claims)
}

type funcDefTokenModel struct {
	Subject     string                `json:"sub"`
	Expires     int64                 `json:"exp,omitempty"`
	Permissions rfv1beta3.Permissions `json:"permissions,omitempty"`
}

// Valid lets us use the user info as Claim for jwt-go.
// It checks the token expiry.
func (u funcDefTokenModel) Valid() error {
	if u.Expires < jwt.TimeFunc().Unix() {
		return errors.New("token expired")
	}
	return nil
}
