package storage

import (
	"context"
	"fmt"
	"io"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/rs/zerolog"

	appconfig "github.com/baechuer/cityevents/services/media-worker/internal/config"
)

// S3Client wraps the AWS S3 client for MinIO/R2.
type S3Client struct {
	client       *s3.Client
	rawBucket    string
	publicBucket string
	log          zerolog.Logger
}

// NewS3Client creates a new S3 client configured for MinIO or R2.
func NewS3Client(cfg *appconfig.Config, log zerolog.Logger) (*S3Client, error) {
	customResolver := aws.EndpointResolverWithOptionsFunc(func(service, region string, options ...interface{}) (aws.Endpoint, error) {
		return aws.Endpoint{
			URL:               cfg.S3Endpoint,
			HostnameImmutable: true,
		}, nil
	})

	awsCfg, err := config.LoadDefaultConfig(context.Background(),
		config.WithRegion(cfg.S3Region),
		config.WithEndpointResolverWithOptions(customResolver),
		config.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(
			cfg.S3AccessKeyID,
			cfg.S3SecretAccessKey,
			"",
		)),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to load AWS config: %w", err)
	}

	client := s3.NewFromConfig(awsCfg, func(o *s3.Options) {
		o.UsePathStyle = cfg.S3UsePathStyle
	})

	return &S3Client{
		client:       client,
		rawBucket:    cfg.RawBucket,
		publicBucket: cfg.PublicBucket,
		log:          log,
	}, nil
}

// GetObject retrieves an object from the raw bucket.
func (c *S3Client) GetObject(ctx context.Context, objectKey string) (io.ReadCloser, error) {
	out, err := c.client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(c.rawBucket),
		Key:    aws.String(objectKey),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to get object %s: %w", objectKey, err)
	}
	return out.Body, nil
}

// PutPublicObject uploads an object to the public bucket.
func (c *S3Client) PutPublicObject(ctx context.Context, objectKey string, data io.Reader, contentType string, size int64) error {
	_, err := c.client.PutObject(ctx, &s3.PutObjectInput{
		Bucket:        aws.String(c.publicBucket),
		Key:           aws.String(objectKey),
		Body:          data,
		ContentType:   aws.String(contentType),
		ContentLength: aws.Int64(size),
	})
	if err != nil {
		return fmt.Errorf("failed to put object %s: %w", objectKey, err)
	}
	return nil
}

// DeleteRawObject deletes an object from the raw bucket.
func (c *S3Client) DeleteRawObject(ctx context.Context, objectKey string) error {
	_, err := c.client.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: aws.String(c.rawBucket),
		Key:    aws.String(objectKey),
	})
	return err
}
