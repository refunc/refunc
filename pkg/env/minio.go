package env

import (
	"path/filepath"
	"strings"
	"sync"

	minio "github.com/minio/minio-go"
)

var (
	minioClient   *minio.Client
	getClientOnce sync.Once
)

// GlobalMinioClient returns a minio client with default config
func GlobalMinioClient() *minio.Client {
	getClientOnce.Do(func() {
		if GlobalMinioEndpoint == "" {
			panic(`Refunc: missing required ENV key "MINIO_ENDPOINT"`)
		}
		if GlobalBucket == "" {
			panic(`Refunc: missing required ENV key "MINIO_BUCKET"`)
		}

		var err error
		endpoint, isSecure := ParseMinioEndpoint(GlobalMinioEndpoint)
		minioClient, err = minio.New(endpoint, GlobalAccessKey, GlobalSecretKey, isSecure)
		if err != nil {
			panic(err)
		}
	})

	return minioClient
}

var (
	minioPubClient   *minio.Client
	getPubClientOnce sync.Once
)

// GlobalMinioPublicClient returns a minio client that mount at public endpoint
func GlobalMinioPublicClient() *minio.Client {
	getClientOnce.Do(func() {
		if GlobalMinioPublicEndpoint == "" {
			panic(`Refunc: missing required ENV key "MINIO_ENDPOINT"`)
		}
		if GlobalBucket == "" {
			panic(`Refunc: missing required ENV key "MINIO_BUCKET"`)
		}

		var err error
		endpoint, isSecure := ParseMinioEndpoint(GlobalMinioPublicEndpoint)
		minioClient, err = minio.New(endpoint, GlobalAccessKey, GlobalSecretKey, isSecure)
		if err != nil {
			panic(err)
		}
	})

	return minioClient
}

// KeyWithinScope returns full path key prefixed with scope root under bucket
func KeyWithinScope(key string) string {
	return filepath.Join(GlobalScopeRoot, key)
}

// ParseMinioEndpoint returns endpoint and if current endpoint requires tls
func ParseMinioEndpoint(rawEndpoint string) (endpoint string, isSecure bool) {
	if rawEndpoint == "" {
		return
	}
	switch {
	case strings.HasPrefix(rawEndpoint, "http://"):
		endpoint = rawEndpoint[7:]
		return
	case strings.HasPrefix(rawEndpoint, "https://"):
		endpoint = rawEndpoint[8:]
		isSecure = true
		return
	}
	endpoint = rawEndpoint
	return
}
