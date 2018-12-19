package runtime

import (
	"net/url"
	"strings"
	"time"

	"github.com/refunc/refunc/pkg/env"
)

func genDownloadURL(path string, expires time.Time) (string, error) {
	u, err := url.Parse(path)
	if err != nil {
		return "", err
	}
	bucket := u.Host
	u, err = env.GlobalMinioClient().PresignedGetObject(bucket, strings.TrimLeft(u.Path, "/"), expires.Sub(time.Now()), url.Values{})
	if err != nil {
		return "", err
	}
	return u.String(), nil
}
