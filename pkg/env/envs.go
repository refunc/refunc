package env

import (
	"os"
	"strings"
)

// Well known credentials load from env during initializing
var (
	GlobalToken     string
	GlobalAccessKey string
	GlobalSecretKey string

	GlobalMinioEndpoint       string
	GlobalMinioPublicEndpoint string
	GlobalBucket              string
	GlobalScopeRoot           string

	GlobalNATSEndpoint string
)

func init() {
	loadEnvs()
}

// RefreshEnvs reload env in case of os.Setenv
func RefreshEnvs() {
	loadEnvs()
}

func loadEnvs() {
	GlobalToken = os.Getenv("ACCESS_TOKEN")
	if GlobalToken == "" {
		GlobalToken = os.Getenv("REFUNC_TOKEN")
	}

	GlobalAccessKey, GlobalSecretKey = os.Getenv("MINIO_ACCESS_KEY"), os.Getenv("MINIO_SECRET_KEY")
	if GlobalAccessKey == "" {
		GlobalAccessKey = os.Getenv("REFUNC_ACCESS_KEY")
	}
	if GlobalSecretKey == "" {
		GlobalSecretKey = os.Getenv("REFUNC_SECRET_KEY")
	}

	GlobalMinioEndpoint = os.Getenv("MINIO_ENDPOINT")
	if GlobalMinioEndpoint == "" {
		GlobalMinioEndpoint = os.Getenv("REFUNC_MINIO_ENDPOINT")
	}
	if GlobalMinioEndpoint == "" {
		GlobalMinioEndpoint = "http://10.43.99.201"
	}
	GlobalMinioPublicEndpoint = os.Getenv("MINIO_PUBLIC_ENDPOINT")
	if GlobalMinioPublicEndpoint == "" {
		GlobalMinioPublicEndpoint = os.Getenv("REFUNC_MINIO_PUBLIC_ENDPOINT")
	}
	if GlobalMinioPublicEndpoint == "" {
		GlobalMinioPublicEndpoint = "https://s3.refunc.io"
	}

	GlobalBucket = os.Getenv("MINIO_BUCKET")
	if GlobalBucket == "" {
		GlobalBucket = os.Getenv("REFUNC_MINIO_BUCKET")
	}
	GlobalScopeRoot = os.Getenv("MINIO_SCOPE")
	if GlobalScopeRoot == "" {
		GlobalScopeRoot = os.Getenv("REFUNC_MINIO_SCOPE")
	}
	GlobalScopeRoot = strings.TrimPrefix(GlobalScopeRoot, "/")

	GlobalNATSEndpoint = os.Getenv("NATS_ENDPOINT")
	if GlobalNATSEndpoint == "" {
		GlobalNATSEndpoint = os.Getenv("REFUNC_NATS_ENDPOINT")
	}
	if GlobalNATSEndpoint == "" {
		GlobalNATSEndpoint = "nats.refunc.svc.cluster.local:4222"
	}
}
