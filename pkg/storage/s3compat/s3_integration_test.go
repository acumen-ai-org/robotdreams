//go:build integration

package s3compat

import (
	"context"
	"fmt"
	"net/url"
	"sync/atomic"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	tcminio "github.com/testcontainers/testcontainers-go/modules/minio"

	"github.com/acumen-ai-org/robotdreams/pkg/storage"
	"github.com/acumen-ai-org/robotdreams/pkg/storage/testsuite"
)

func startMinio(t *testing.T) (client *s3.Client, endpoint, accessKey, secretKey string) {
	t.Helper()
	ctx := context.Background()

	container, err := tcminio.Run(ctx, "minio/minio:RELEASE.2024-01-16T16-07-38Z")
	if err != nil {
		t.Fatalf("starting MinIO container: %v", err)
	}
	t.Cleanup(func() {
		if err := container.Terminate(context.Background()); err != nil {
			t.Logf("terminating MinIO container: %v", err)
		}
	})

	connStr, err := container.ConnectionString(ctx)
	if err != nil {
		t.Fatalf("getting MinIO connection string: %v", err)
	}
	endpoint = "http://" + connStr
	accessKey = container.Username
	secretKey = container.Password

	awsCfg, err := awsconfig.LoadDefaultConfig(ctx,
		awsconfig.WithRegion(defaultRegion),
		awsconfig.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(accessKey, secretKey, "")),
	)
	if err != nil {
		t.Fatalf("loading AWS config for MinIO client: %v", err)
	}
	client = s3.NewFromConfig(awsCfg, func(o *s3.Options) {
		o.BaseEndpoint = aws.String(endpoint)
		o.UsePathStyle = true
	})
	return client, endpoint, accessKey, secretKey
}

var bucketCounter atomic.Int64

func createBucket(t *testing.T, client *s3.Client, versioned bool) string {
	t.Helper()
	ctx := context.Background()

	bucket := fmt.Sprintf("rd-test-%d-%d", bucketCounter.Add(1), 1)
	if _, err := client.CreateBucket(ctx, &s3.CreateBucketInput{Bucket: aws.String(bucket)}); err != nil {
		t.Fatalf("creating bucket %q: %v", bucket, err)
	}

	if versioned {
		_, err := client.PutBucketVersioning(ctx, &s3.PutBucketVersioningInput{
			Bucket: aws.String(bucket),
			VersioningConfiguration: &types.VersioningConfiguration{
				Status: types.BucketVersioningStatusEnabled,
			},
		})
		if err != nil {
			t.Fatalf("enabling versioning on bucket %q: %v", bucket, err)
		}
	}

	return bucket
}

func backendURI(bucket, endpoint, accessKey, secretKey string) string {
	q := url.Values{}
	q.Set("endpoint", endpoint)
	q.Set("region", defaultRegion)
	q.Set("use-path-style", "true")
	q.Set("access-key", accessKey)
	q.Set("secret-key", secretKey)
	return fmt.Sprintf("s3://%s?%s", bucket, q.Encode())
}

func TestConformanceUnversionedBucket(t *testing.T) {
	client, endpoint, accessKey, secretKey := startMinio(t)

	testsuite.RunConformance(t, func() storage.StorageBackend {
		bucket := createBucket(t, client, false)
		b, err := New(context.Background(), backendURI(bucket, endpoint, accessKey, secretKey))
		if err != nil {
			t.Fatalf("New: %v", err)
		}
		return b
	})
}

func TestConformanceVersionedBucket(t *testing.T) {
	client, endpoint, accessKey, secretKey := startMinio(t)

	testsuite.RunConformance(t, func() storage.StorageBackend {
		bucket := createBucket(t, client, true)
		b, err := New(context.Background(), backendURI(bucket, endpoint, accessKey, secretKey))
		if err != nil {
			t.Fatalf("New: %v", err)
		}
		return b
	})
}

func TestVersioningEnabledDetected(t *testing.T) {
	client, endpoint, accessKey, secretKey := startMinio(t)
	ctx := context.Background()

	plain := createBucket(t, client, false)
	versioned := createBucket(t, client, true)

	bPlain, err := New(ctx, backendURI(plain, endpoint, accessKey, secretKey))
	if err != nil {
		t.Fatalf("New (plain): %v", err)
	}
	defer bPlain.Close()
	if bPlain.versioningEnabled {
		t.Fatalf("expected versioningEnabled=false for a plain bucket")
	}

	bVersioned, err := New(ctx, backendURI(versioned, endpoint, accessKey, secretKey))
	if err != nil {
		t.Fatalf("New (versioned): %v", err)
	}
	defer bVersioned.Close()
	if !bVersioned.versioningEnabled {
		t.Fatalf("expected versioningEnabled=true for a versioned bucket")
	}
}
