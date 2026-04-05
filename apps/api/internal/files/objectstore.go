package files

import (
	"context"
	"errors"
	"fmt"
	"bytes"
	"io"
	"net/url"
	"os"
	"strconv"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
)

type ObjectStore struct {
	Bucket string
	S3     *s3.Client
	Sign   *s3.PresignClient
}

type Config struct {
	Bucket         string
	Region         string
	Endpoint       string
	AccessKeyID    string
	SecretAccessKey string
	ForcePathStyle bool
}

func FromEnv() (Config, error) {
	c := Config{
		Bucket:          os.Getenv("FILES_S3_BUCKET"),
		Region:          os.Getenv("FILES_S3_REGION"),
		Endpoint:        os.Getenv("FILES_S3_ENDPOINT"),
		AccessKeyID:     os.Getenv("FILES_S3_ACCESS_KEY"),
		SecretAccessKey: os.Getenv("FILES_S3_SECRET_KEY"),
	}
	if c.Bucket == "" {
		c.Bucket = "hyperspeed-files"
	}
	if c.Region == "" {
		c.Region = "us-east-1"
	}
	if v := os.Getenv("FILES_S3_FORCE_PATH_STYLE"); v != "" {
		b, _ := strconv.ParseBool(v)
		c.ForcePathStyle = b
	}
	return c, nil
}

func New(ctx context.Context, c Config) (*ObjectStore, error) {
	loadOpts := []func(*config.LoadOptions) error{
		config.WithRegion(c.Region),
	}
	if c.AccessKeyID != "" && c.SecretAccessKey != "" {
		loadOpts = append(loadOpts, config.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(c.AccessKeyID, c.SecretAccessKey, "")))
	}
	awsCfg, err := config.LoadDefaultConfig(ctx, loadOpts...)
	if err != nil {
		return nil, err
	}

	if c.Endpoint != "" {
		u, err := url.Parse(c.Endpoint)
		if err != nil {
			return nil, fmt.Errorf("invalid FILES_S3_ENDPOINT: %w", err)
		}
		_ = u
	}

	s3c := s3.NewFromConfig(awsCfg, func(o *s3.Options) {
		if c.Endpoint != "" {
			o.EndpointResolver = s3.EndpointResolverFromURL(c.Endpoint)
		}
		if c.ForcePathStyle {
			o.UsePathStyle = true
		}
	})

	return &ObjectStore{
		Bucket: c.Bucket,
		S3:     s3c,
		Sign:   s3.NewPresignClient(s3c),
	}, nil
}

func (s *ObjectStore) EnsureBucket(ctx context.Context) error {
	_, err := s.S3.HeadBucket(ctx, &s3.HeadBucketInput{Bucket: aws.String(s.Bucket)})
	if err == nil {
		return nil
	}
	var nfe *types.NotFound
	if errors.As(err, &nfe) {
		_, err = s.S3.CreateBucket(ctx, &s3.CreateBucketInput{Bucket: aws.String(s.Bucket)})
		return err
	}
	// Some S3-compatible providers return generic errors on HeadBucket; attempt create.
	_, cerr := s.S3.CreateBucket(ctx, &s3.CreateBucketInput{Bucket: aws.String(s.Bucket)})
	if cerr == nil {
		return nil
	}
	return err
}

func (s *ObjectStore) PresignPut(ctx context.Context, key string, contentType string, expires time.Duration) (string, error) {
	in := &s3.PutObjectInput{
		Bucket: aws.String(s.Bucket),
		Key:    aws.String(key),
	}
	if contentType != "" {
		in.ContentType = aws.String(contentType)
	}
	out, err := s.Sign.PresignPutObject(ctx, in, func(po *s3.PresignOptions) {
		po.Expires = expires
	})
	if err != nil {
		return "", err
	}
	return out.URL, nil
}

func (s *ObjectStore) PresignGet(ctx context.Context, key string, expires time.Duration) (string, error) {
	out, err := s.Sign.PresignGetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(s.Bucket),
		Key:    aws.String(key),
	}, func(po *s3.PresignOptions) {
		po.Expires = expires
	})
	if err != nil {
		return "", err
	}
	return out.URL, nil
}

func (s *ObjectStore) Put(ctx context.Context, key string, contentType string, body io.Reader, size *int64) error {
	in := &s3.PutObjectInput{
		Bucket: aws.String(s.Bucket),
		Key:    aws.String(key),
		Body:   body,
	}
	if contentType != "" {
		in.ContentType = aws.String(contentType)
	}
	if size != nil {
		in.ContentLength = size
	}
	_, err := s.S3.PutObject(ctx, in)
	return err
}

func (s *ObjectStore) HeadSize(ctx context.Context, key string) (int64, error) {
	out, err := s.S3.HeadObject(ctx, &s3.HeadObjectInput{
		Bucket: aws.String(s.Bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		return 0, err
	}
	if out.ContentLength == nil {
		return 0, nil
	}
	return *out.ContentLength, nil
}

// DownloadToPath streams the object into path, reading at most maxBytes.
func (s *ObjectStore) DownloadToPath(ctx context.Context, key, path string, maxBytes int64) error {
	out, err := s.S3.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(s.Bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		return err
	}
	defer out.Body.Close()
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	if maxBytes <= 0 {
		maxBytes = 1 << 30
	}
	_, err = io.Copy(f, io.LimitReader(out.Body, maxBytes))
	return err
}

// GetObjectStream returns the object body; caller must close ReadCloser.
func (s *ObjectStore) GetObjectStream(ctx context.Context, key string) (io.ReadCloser, int64, error) {
	out, err := s.S3.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(s.Bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		return nil, 0, err
	}
	var n int64
	if out.ContentLength != nil {
		n = *out.ContentLength
	}
	return out.Body, n, nil
}

func (s *ObjectStore) GetBytes(ctx context.Context, key string, limit int64) ([]byte, error) {
	out, err := s.S3.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(s.Bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		return nil, err
	}
	defer out.Body.Close()
	if limit <= 0 {
		limit = 1 << 20 // 1MB default safety cap
	}
	b, err := io.ReadAll(io.LimitReader(out.Body, limit))
	if err != nil {
		return nil, err
	}
	// If truncated, caller can decide later; for MVP we just cap.
	return b, nil
}

func (s *ObjectStore) Delete(ctx context.Context, key string) error {
	_, err := s.S3.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: aws.String(s.Bucket),
		Key:    aws.String(key),
	})
	return err
}

func (s *ObjectStore) PutString(ctx context.Context, key string, contentType string, content string) error {
	b := []byte(content)
	size := int64(len(b))
	return s.Put(ctx, key, contentType, bytes.NewReader(b), &size)
}

