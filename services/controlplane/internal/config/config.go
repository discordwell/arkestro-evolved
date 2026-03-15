package config

import (
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	HTTPAddr             string
	DatabaseURL          string
	CatalogPath          string
	ObjectStore          string
	ArtifactRoot         string
	S3Endpoint           string
	S3Bucket             string
	S3Region             string
	S3AccessKeyID        string
	S3SecretAccessKey    string
	WorkerPoll           time.Duration
	DefaultOrgID         string
	DefaultOrgName       string
	DefaultAdminEmail    string
	DefaultAdminPassword string
	DefaultAdminName     string
}

func Load() Config {
	return Config{
		HTTPAddr:             getenv("EVO_HTTP_ADDR", ":8080"),
		DatabaseURL:          getenv("EVO_DATABASE_URL", "postgres://postgres@localhost:5432/postgres?sslmode=disable"),
		CatalogPath:          getenv("EVO_CATALOG_PATH", "catalog/runbooks.json"),
		ObjectStore:          getenv("EVO_OBJECT_STORE", "fs"),
		ArtifactRoot:         getenv("EVO_ARTIFACT_ROOT", "services/controlplane/data/artifacts"),
		S3Endpoint:           getenv("EVO_S3_ENDPOINT", ""),
		S3Bucket:             getenv("EVO_S3_BUCKET", "evo-artifacts"),
		S3Region:             getenv("EVO_S3_REGION", "us-east-1"),
		S3AccessKeyID:        getenv("EVO_S3_ACCESS_KEY_ID", ""),
		S3SecretAccessKey:    getenv("EVO_S3_SECRET_ACCESS_KEY", ""),
		WorkerPoll:           getDuration("EVO_WORKER_POLL_MS", 1000),
		DefaultOrgID:         getenv("EVO_DEFAULT_ORG_ID", "11111111-1111-1111-1111-111111111111"),
		DefaultOrgName:       getenv("EVO_DEFAULT_ORG_NAME", "Default Org"),
		DefaultAdminEmail:    getenv("EVO_DEFAULT_ADMIN_EMAIL", "admin@evo.local"),
		DefaultAdminPassword: getenv("EVO_DEFAULT_ADMIN_PASSWORD", "changeme"),
		DefaultAdminName:     getenv("EVO_DEFAULT_ADMIN_NAME", "Platform Admin"),
	}
}

func getenv(key, fallback string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return fallback
}

func getDuration(key string, fallbackMS int) time.Duration {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return time.Duration(fallbackMS) * time.Millisecond
	}
	ms, err := strconv.Atoi(v)
	if err != nil {
		return time.Duration(fallbackMS) * time.Millisecond
	}
	return time.Duration(ms) * time.Millisecond
}
