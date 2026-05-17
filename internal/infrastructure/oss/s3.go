// Package oss contains storage adapters that satisfy domain.OSSProvider.
//
// The S3 adapter targets any S3-compatible endpoint via aws-sdk-go-v2.
package oss

import (
	"bytes"
	"context"
	"fmt"
	"net/url"
	"strings"

	awsv2 "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"

	"github.com/mmmy/snapgo/internal/domain"
)

// S3Provider implements domain.OSSProvider using aws-sdk-go-v2.
//
// We construct one client per Provider instance because the S3 client is
// goroutine-safe and the configuration is immutable for the lifetime of the
// provider.
type S3Provider struct {
	cfg    domain.S3Config
	client *s3.Client
}

// NewS3Provider builds an *S3Provider from the supplied user configuration.
//
// The endpoint is plumbed through BaseEndpoint + UsePathStyle so any
// S3-compatible service (MinIO, R2, B2, Aliyun OSS s3 endpoint) works
// without us writing per-vendor branches.
func NewS3Provider(cfg domain.S3Config) (*S3Provider, error) {
	if cfg.Endpoint == "" || cfg.Bucket == "" || cfg.AccessKeyID == "" || cfg.SecretAccessKey == "" {
		return nil, fmt.Errorf("s3 config: endpoint/bucket/accessKeyId/secretAccessKey are required")
	}
	region := cfg.Region
	if region == "" {
		region = "us-east-1"
	}
	awsCfg := awsv2.Config{
		Region: region,
		Credentials: credentials.NewStaticCredentialsProvider(
			cfg.AccessKeyID, cfg.SecretAccessKey, "",
		),
	}
	endpoint := cfg.Endpoint
	client := s3.NewFromConfig(awsCfg, func(o *s3.Options) {
		o.BaseEndpoint = awsv2.String(endpoint)
		o.UsePathStyle = cfg.UsePathStyle
	})
	return &S3Provider{cfg: cfg, client: client}, nil
}

// Name identifies this provider in logs and history records.
func (p *S3Provider) Name() string { return "s3" }

// Upload pushes the bytes to the configured bucket and returns a public URL.
//
// We pass the body via bytes.Reader so the SDK can compute Content-Length
// and signed checksum without buffering the data twice.
func (p *S3Provider) Upload(ctx context.Context, key string, data []byte, contentType string) (string, error) {
	if contentType == "" {
		contentType = "image/png"
	}
	_, err := p.client.PutObject(ctx, &s3.PutObjectInput{
		Bucket:      awsv2.String(p.cfg.Bucket),
		Key:         awsv2.String(key),
		Body:        bytes.NewReader(data),
		ContentType: awsv2.String(contentType),
	})
	if err != nil {
		return "", fmt.Errorf("s3 put object: %w", err)
	}
	return p.BuildPublicURL(key), nil
}

// BuildPublicURL constructs the URL that the user pastes into Markdown.
//
// Priority:
//  1. PublicURLBase if user filled it (best-fit for CDN-fronted buckets);
//  2. {Endpoint}/{Bucket}/{Key} for path-style;
//  3. virtual-hosted-style fallback ({bucket}.host).
func (p *S3Provider) BuildPublicURL(key string) string {
	if p.cfg.PublicURLBase != "" {
		base := strings.TrimRight(p.cfg.PublicURLBase, "/")
		return base + "/" + key
	}
	endpoint := strings.TrimRight(p.cfg.Endpoint, "/")
	if p.cfg.UsePathStyle {
		return endpoint + "/" + p.cfg.Bucket + "/" + key
	}
	// Virtual-hosted-style: replace host with {bucket}.{host}
	if u, err := url.Parse(endpoint); err == nil && u.Host != "" {
		u.Host = p.cfg.Bucket + "." + u.Host
		u.Path = "/" + key
		return u.String()
	}
	return endpoint + "/" + p.cfg.Bucket + "/" + key
}

// TestConnection performs a small put + delete to verify credentials and
// bucket reachability. Used by the "Test connection" button in settings.
func (p *S3Provider) TestConnection(ctx context.Context) error {
	probeKey := strings.TrimRight(p.cfg.PathPrefix, "/") + "/.snapgo-probe"
	probeKey = strings.TrimLeft(probeKey, "/")
	if _, err := p.client.PutObject(ctx, &s3.PutObjectInput{
		Bucket:      awsv2.String(p.cfg.Bucket),
		Key:         awsv2.String(probeKey),
		Body:        bytes.NewReader([]byte("snapgo")),
		ContentType: awsv2.String("text/plain"),
	}); err != nil {
		return fmt.Errorf("probe put: %w", err)
	}
	if _, err := p.client.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: awsv2.String(p.cfg.Bucket),
		Key:    awsv2.String(probeKey),
	}); err != nil {
		return fmt.Errorf("probe delete: %w", err)
	}
	return nil
}
