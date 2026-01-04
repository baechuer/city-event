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

	appconfig "github.com/baechuer/cityevents/services/media-service/internal/config"
)

// S3Client wraps the AWS S3 client for MinIO/R2.
type S3Client struct {
	client            *s3.Client
	presigner         *s3.PresignClient
	externalPresigner *s3.PresignClient // For browser-accessible presigned URLs
	rawBucket         string
	publicBucket      string
	cfg               *appconfig.Config
	log               zerolog.Logger
}

// NewS3Client creates a new S3 client configured for MinIO or R2.
func NewS3Client(cfg *appconfig.Config, log zerolog.Logger) (*S3Client, error) {
	// Internal client for server-to-server operations
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

	// External presigner for browser uploads (uses external endpoint for correct signature)
	externalEndpoint := cfg.S3ExternalEndpoint
	if externalEndpoint == "" {
		externalEndpoint = cfg.S3Endpoint
	}
	externalResolver := aws.EndpointResolverWithOptionsFunc(func(service, region string, options ...interface{}) (aws.Endpoint, error) {
		return aws.Endpoint{
			URL:               externalEndpoint,
			HostnameImmutable: true,
		}, nil
	})
	externalAwsCfg, err := config.LoadDefaultConfig(context.Background(),
		config.WithRegion(cfg.S3Region),
		config.WithEndpointResolverWithOptions(externalResolver),
		config.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(
			cfg.S3AccessKeyID,
			cfg.S3SecretAccessKey,
			"",
		)),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to load external AWS config: %w", err)
	}
	externalClient := s3.NewFromConfig(externalAwsCfg, func(o *s3.Options) {
		o.UsePathStyle = cfg.S3UsePathStyle
	})

	return &S3Client{
		client:            client,
		presigner:         s3.NewPresignClient(client),
		externalPresigner: s3.NewPresignClient(externalClient),
		rawBucket:         cfg.RawBucket,
		publicBucket:      cfg.PublicBucket,
		cfg:               cfg,
		log:               log,
	}, nil
}

// GeneratePresignedPutURL creates a presigned URL for uploading to raw bucket.
// Uses externalPresigner so the signature is valid for browser access via external endpoint.
func (c *S3Client) GeneratePresignedPutURL(ctx context.Context, objectKey string, contentLengthLimit int64) (string, error) {
	// Note: ContentLength is NOT included to allow flexible file sizes
	// Size validation happens in CompleteUpload after the file is uploaded
	req, err := c.externalPresigner.PresignPutObject(ctx, &s3.PutObjectInput{
		Bucket: aws.String(c.rawBucket),
		Key:    aws.String(objectKey),
	}, s3.WithPresignExpires(c.cfg.PresignTTL))
	if err != nil {
		return "", fmt.Errorf("failed to presign PUT: %w", err)
	}
	return req.URL, nil
}

// ObjectExists checks if an object exists in the raw bucket.
func (c *S3Client) ObjectExists(ctx context.Context, objectKey string) (bool, int64, error) {
	out, err := c.client.HeadObject(ctx, &s3.HeadObjectInput{
		Bucket: aws.String(c.rawBucket),
		Key:    aws.String(objectKey),
	})
	if err != nil {
		// Check if it's a "not found" error
		return false, 0, nil
	}
	return true, aws.ToInt64(out.ContentLength), nil
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

// EnsureBuckets creates the raw and public buckets if they don't exist.
// Also sets public read policy on the public bucket.
func (c *S3Client) EnsureBuckets(ctx context.Context) error {
	buckets := []string{c.rawBucket, c.publicBucket}
	for _, bucket := range buckets {
		_, err := c.client.HeadBucket(ctx, &s3.HeadBucketInput{
			Bucket: aws.String(bucket),
		})
		if err != nil {
			c.log.Info().Str("bucket", bucket).Msg("creating bucket")
			_, createErr := c.client.CreateBucket(ctx, &s3.CreateBucketInput{
				Bucket: aws.String(bucket),
			})
			if createErr != nil {
				return fmt.Errorf("failed to create bucket %s: %w", bucket, createErr)
			}
		}
	}

	// Set public read policy on public bucket
	policy := fmt.Sprintf(`{
		"Version": "2012-10-17",
		"Statement": [{
			"Effect": "Allow",
			"Principal": {"AWS": ["*"]},
			"Action": ["s3:GetObject"],
			"Resource": ["arn:aws:s3:::%s/*"]
		}]
	}`, c.publicBucket)

	_, err := c.client.PutBucketPolicy(ctx, &s3.PutBucketPolicyInput{
		Bucket: aws.String(c.publicBucket),
		Policy: aws.String(policy),
	})
	if err != nil {
		c.log.Warn().Err(err).Msg("failed to set public bucket policy")
		// Don't fail - bucket might already have policy
	} else {
		c.log.Info().Str("bucket", c.publicBucket).Msg("set public read policy")
	}

	return nil
}

// PublicURL returns the public URL for a derived object.
func (c *S3Client) PublicURL(objectKey string) string {
	return c.cfg.CDNBaseURL + "/" + objectKey
}
