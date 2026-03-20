package config

import (
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"
)

type Config struct {
	// HTTP
	Port              int
	VerificationToken string

	// Redis - cache
	RedisURL  string
	RedisCert string

	// Redis - queue (BullMQ)
	MQRedisURL  string
	MQRedisCert string

	// Browser
	ChromiumPath   string
	FirefoxPath    string
	UseFirefox     bool
	LaunchHeadless bool

	// Object storage
	// BlobStorageURL is a gocloud.dev blob URL that selects the backend:
	//   gs://bucket                          → GCS (Application Default Credentials)
	//   s3://bucket?region=us-east-1         → AWS S3
	//   s3://bucket?endpoint=http://minio:9000&use_path_style=true&disable_https=true&region=us-east-1
	//                                         → MinIO
	// When empty, Config.BlobURL() chooses a MinIO-compatible s3:// fallback for
	// self-hosted deployments, while preserving legacy gs:// behavior when GCS
	// credentials are explicitly configured.
	BlobStorageURL string

	// Legacy GCS settings kept for backward compatibility.
	// Prefer BLOB_STORAGE_URL for new deployments.
	GCSUploadBucket    string
	GCSKeyFilePath     string
	SkipUploadOriginal bool
	S3EndpointURL      string
	S3Region           string

	EnableGraphQLPlayground bool

	// Import metrics
	ImporterMetricsCollectorURL string
	JWTSecret                   string

	// Domain blocking
	MaxFeedFetchFailures int

	// Database
	DatabaseURL string
}

type APIConfig struct {
	// HTTP
	Port int

	// Redis - cache
	RedisURL  string
	RedisCert string

	// Redis - queue (BullMQ)
	MQRedisURL  string
	MQRedisCert string

	// Object storage
	BlobStorageURL string

	// Legacy GCS settings kept for backward compatibility.
	// Prefer BLOB_STORAGE_URL for new deployments.
	GCSUploadBucket    string
	GCSKeyFilePath     string
	SkipUploadOriginal bool
	S3EndpointURL      string
	S3Region           string

	EnableGraphQLPlayground bool

	// Import metrics
	ImporterMetricsCollectorURL string
	JWTSecret                   string

	// Domain blocking
	MaxFeedFetchFailures int

	// Database
	DatabaseURL string
}

func Load() *Config {
	apiCfg := loadAPIConfig()

	return &Config{
		Port:              apiCfg.Port,
		VerificationToken: os.Getenv("VERIFICATION_TOKEN"),

		RedisURL:  apiCfg.RedisURL,
		RedisCert: apiCfg.RedisCert,

		MQRedisURL:  apiCfg.MQRedisURL,
		MQRedisCert: apiCfg.MQRedisCert,

		ChromiumPath:   envDefault("CHROMIUM_PATH", "/usr/bin/chromium"),
		FirefoxPath:    envDefault("FIREFOX_PATH", "/usr/bin/firefox"),
		UseFirefox:     os.Getenv("USE_FIREFOX") == "true",
		LaunchHeadless: os.Getenv("LAUNCH_HEADLESS") == "true",

		BlobStorageURL: apiCfg.BlobStorageURL,

		GCSUploadBucket:    apiCfg.GCSUploadBucket,
		GCSKeyFilePath:     apiCfg.GCSKeyFilePath,
		SkipUploadOriginal: apiCfg.SkipUploadOriginal,
		S3EndpointURL:      apiCfg.S3EndpointURL,
		S3Region:           apiCfg.S3Region,

		EnableGraphQLPlayground: apiCfg.EnableGraphQLPlayground,

		ImporterMetricsCollectorURL: apiCfg.ImporterMetricsCollectorURL,
		JWTSecret:                   apiCfg.JWTSecret,

		MaxFeedFetchFailures: apiCfg.MaxFeedFetchFailures,

		DatabaseURL: apiCfg.DatabaseURL,
	}
}

func LoadAPI() *APIConfig {
	cfg := loadAPIConfig()
	return &cfg
}

func loadAPIConfig() APIConfig {
	return APIConfig{
		Port: envInt("PORT", 3002),

		RedisURL:  os.Getenv("REDIS_URL"),
		RedisCert: os.Getenv("REDIS_CERT"),

		MQRedisURL:  os.Getenv("MQ_REDIS_URL"),
		MQRedisCert: os.Getenv("MQ_REDIS_CERT"),

		BlobStorageURL: os.Getenv("BLOB_STORAGE_URL"),

		GCSUploadBucket:    envDefault("GCS_UPLOAD_BUCKET", "omnivore-files"),
		GCSKeyFilePath:     os.Getenv("GCS_UPLOAD_SA_KEY_FILE_PATH"),
		SkipUploadOriginal: os.Getenv("SKIP_UPLOAD_ORIGINAL") == "true",
		S3EndpointURL:      envDefault("AWS_S3_ENDPOINT_URL", "http://minio:9000"),
		S3Region:           envDefault("AWS_REGION", "us-east-1"),

		EnableGraphQLPlayground: os.Getenv("ENABLE_GRAPHQL_PLAYGROUND") == "true",

		ImporterMetricsCollectorURL: os.Getenv("IMPORTER_METRICS_COLLECTOR_URL"),
		JWTSecret:                   os.Getenv("JWT_SECRET"),

		MaxFeedFetchFailures: envInt("MAX_FEED_FETCH_FAILURES", 10),

		DatabaseURL: BuildDatabaseURL(),
	}
}

// BlobURL returns the effective gocloud.dev blob URL to open.
// If BLOB_STORAGE_URL is set it is returned as-is.
// If legacy GCS credentials are configured, a gs:// URL is constructed from
// GCS_UPLOAD_BUCKET for backward compatibility.
// Otherwise a MinIO-compatible s3:// URL is constructed from the bucket name,
// AWS_S3_ENDPOINT_URL, and AWS_REGION so self-hosted deployments default to
// local object storage.
func (c *Config) BlobURL() string {
	return buildBlobURL(c.BlobStorageURL, c.GCSUploadBucket, c.GCSKeyFilePath, c.S3EndpointURL, c.S3Region)
}

// BlobURL returns the effective gocloud.dev blob URL to open for the API service.
func (c *APIConfig) BlobURL() string {
	return buildBlobURL(c.BlobStorageURL, c.GCSUploadBucket, c.GCSKeyFilePath, c.S3EndpointURL, c.S3Region)
}

func (c *Config) CacheRedisConfig() (string, string) {
	return c.RedisURL, c.RedisCert
}

func (c *Config) MQRedisConfig() (string, string) {
	return c.MQRedisURL, c.MQRedisCert
}

func (c *APIConfig) CacheRedisConfig() (string, string) {
	return c.RedisURL, c.RedisCert
}

func (c *APIConfig) MQRedisConfig() (string, string) {
	return c.MQRedisURL, c.MQRedisCert
}

func buildBlobURL(blobStorageURL, bucket, gcsKeyFilePath, s3EndpointURL, s3Region string) string {
	if blobStorageURL != "" {
		return blobStorageURL
	}
	if gcsKeyFilePath != "" {
		return "gs://" + bucket
	}

	query := url.Values{}
	query.Set("endpoint", s3EndpointURL)
	query.Set("region", s3Region)
	query.Set("use_path_style", "true")
	query.Set("disable_https", strconv.FormatBool(strings.HasPrefix(strings.ToLower(s3EndpointURL), "http://")))

	return "s3://" + bucket + "?" + query.Encode()
}

func envDefault(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func envInt(key string, def int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return def
}

func BuildDatabaseURL() string {
	if v := os.Getenv("DATABASE_URL"); v != "" {
		return v
	}
	if v := os.Getenv("PG_DSN"); v != "" {
		return v
	}
	host := envDefault("PG_HOST", "localhost")
	user := envDefault("PG_USER", "app_user")
	pass := envDefault("PG_PASSWORD", "app_pass")
	db := envDefault("PG_DB", "omnivore")
	port := envDefault("PG_PORT", "5432")
	return fmt.Sprintf("postgres://%s:%s@%s:%s/%s?sslmode=disable", user, pass, host, port, db)
}
