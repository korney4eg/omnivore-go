package config

import (
	"net/url"
	"testing"
)

func TestBlobURLPrefersExplicitURL(t *testing.T) {
	cfg := &Config{
		BlobStorageURL: "s3://custom-bucket?region=us-west-2",
	}

	if got := cfg.BlobURL(); got != "s3://custom-bucket?region=us-west-2" {
		t.Fatalf("BlobURL() = %q, want explicit blob URL", got)
	}
}

func TestBlobURLDefaultsToMinIO(t *testing.T) {
	cfg := &Config{
		GCSUploadBucket: "omnivore",
		S3EndpointURL:   "http://minio:9000",
		S3Region:        "us-east-1",
	}

	got := cfg.BlobURL()
	parsed, err := url.Parse(got)
	if err != nil {
		t.Fatalf("BlobURL() returned invalid URL %q: %v", got, err)
	}

	if parsed.Scheme != "s3" {
		t.Fatalf("scheme = %q, want s3", parsed.Scheme)
	}
	if parsed.Host != "omnivore" {
		t.Fatalf("bucket = %q, want omnivore", parsed.Host)
	}

	query := parsed.Query()
	if query.Get("endpoint") != "http://minio:9000" {
		t.Fatalf("endpoint = %q, want MinIO endpoint", query.Get("endpoint"))
	}
	if query.Get("region") != "us-east-1" {
		t.Fatalf("region = %q, want us-east-1", query.Get("region"))
	}
	if query.Get("use_path_style") != "true" {
		t.Fatalf("use_path_style = %q, want true", query.Get("use_path_style"))
	}
	if query.Get("disable_https") != "true" {
		t.Fatalf("disable_https = %q, want true", query.Get("disable_https"))
	}
}

func TestBlobURLKeepsLegacyGCSFallbackWhenKeyConfigured(t *testing.T) {
	cfg := &Config{
		GCSUploadBucket: "omnivore",
		GCSKeyFilePath:  "/tmp/gcs-key.json",
		S3EndpointURL:   "http://minio:9000",
		S3Region:        "us-east-1",
	}

	if got := cfg.BlobURL(); got != "gs://omnivore" {
		t.Fatalf("BlobURL() = %q, want legacy gs fallback", got)
	}
}
