// Package s3compat is a storage.StorageBackend on any S3-compatible object store, registered as s3://<bucket>/<prefix>?endpoint=...
package s3compat

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"sort"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	smithy "github.com/aws/smithy-go"
	smithyhttp "github.com/aws/smithy-go/transport/http"

	"github.com/acumen-ai-org/robotdreams/pkg/security"
	"github.com/acumen-ai-org/robotdreams/pkg/storage"
)

const maxRevisionHistory = 10

const metaDirName = ".meta"

const (
	metaKeyUpdatedAt = "rd-updated-at"
	metaKeyUpdatedBy = "rd-updated-by"
)

// Backend is a storage.StorageBackend on an S3-compatible bucket, using the object ETag as Revision.
type Backend struct {
	client *s3.Client
	bucket string
	prefix string
	clock  security.Clock

	versioningEnabled bool
}

func init() {
	storage.Register("s3", func(u *url.URL) (storage.StorageBackend, error) {
		return NewFromURL(context.Background(), u)
	})
}

// New parses an s3:// URI and constructs a Backend.
func New(ctx context.Context, uri string) (*Backend, error) {
	u, err := url.Parse(uri)
	if err != nil {
		return nil, fmt.Errorf("s3compat: invalid URI %q: %w", uri, err)
	}
	return NewFromURL(ctx, u)
}

// NewFromURL constructs a Backend from an already-parsed s3:// URI.
func NewFromURL(ctx context.Context, u *url.URL) (*Backend, error) {
	return NewFromURLWithClock(ctx, u, security.RealClock{})
}

// NewFromURLWithClock is NewFromURL with the clock used for UpdatedAt timestamps.
func NewFromURLWithClock(ctx context.Context, u *url.URL, clock security.Clock) (*Backend, error) {
	cfg, err := parseURI(u)
	if err != nil {
		return nil, err
	}

	var optFns []func(*awsconfig.LoadOptions) error
	optFns = append(optFns, awsconfig.WithRegion(cfg.region))
	if cfg.accessKey != "" || cfg.secretKey != "" {
		optFns = append(optFns, awsconfig.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider(cfg.accessKey, cfg.secretKey, ""),
		))
	}

	awsCfg, err := awsconfig.LoadDefaultConfig(ctx, optFns...)
	if err != nil {
		return nil, fmt.Errorf("s3compat: loading AWS config: %w", err)
	}

	client := s3.NewFromConfig(awsCfg, func(o *s3.Options) {
		if cfg.endpoint != "" {
			o.BaseEndpoint = aws.String(cfg.endpoint)
		}
		o.UsePathStyle = cfg.usePathStyle
	})

	b := &Backend{
		client: client,
		bucket: cfg.bucket,
		prefix: cfg.prefix,
		clock:  clock,
	}

	verOut, err := client.GetBucketVersioning(ctx, &s3.GetBucketVersioningInput{
		Bucket: aws.String(cfg.bucket),
	})
	if err != nil {
		return nil, fmt.Errorf("s3compat: checking bucket versioning for %q: %w", cfg.bucket, err)
	}
	b.versioningEnabled = verOut.Status == types.BucketVersioningStatusEnabled

	return b, nil
}

func (b *Backend) resolveKey(objPath string) (string, error) {
	if objPath == "" {
		return "", fmt.Errorf("s3compat: empty path")
	}
	if strings.HasPrefix(objPath, "/") {
		return "", fmt.Errorf("s3compat: invalid path %q: must not be absolute", objPath)
	}
	for _, seg := range strings.Split(objPath, "/") {
		if seg == "" {
			return "", fmt.Errorf("s3compat: invalid path %q: must not contain empty segments", objPath)
		}
		if seg == ".." {
			return "", fmt.Errorf("s3compat: invalid path %q: must not contain \"..\" segments", objPath)
		}
	}

	clean := path.Clean(objPath)
	if clean == "." || clean == ".." || strings.HasPrefix(clean, "../") {
		return "", fmt.Errorf("s3compat: invalid path %q", objPath)
	}
	if clean == metaDirName || strings.HasPrefix(clean, metaDirName+"/") {
		return "", fmt.Errorf("s3compat: invalid path %q: %q is a reserved name", objPath, metaDirName)
	}

	if b.prefix == "" {
		return clean, nil
	}
	return b.prefix + "/" + clean, nil
}

func (b *Backend) pathFromKey(key string) string {
	if b.prefix == "" {
		return key
	}
	return strings.TrimPrefix(key, b.prefix+"/")
}

func (b *Backend) metaDir() string {
	if b.prefix == "" {
		return metaDirName
	}
	return b.prefix + "/" + metaDirName
}

func (b *Backend) sidecarKey(key string) string {
	h := sha256.Sum256([]byte(key))
	return b.metaDir() + "/" + hex.EncodeToString(h[:]) + ".json"
}

func (b *Backend) isUnderMetaDir(key string) bool {
	dir := b.metaDir()
	return key == dir || strings.HasPrefix(key, dir+"/")
}

func trimETag(e *string) string {
	if e == nil {
		return ""
	}
	return strings.Trim(*e, `"`)
}

func quoteETag(rev string) *string {
	q := `"` + rev + `"`
	return &q
}

func isNotFound(err error) bool {
	if err == nil {
		return false
	}
	var nsk *types.NoSuchKey
	if errors.As(err, &nsk) {
		return true
	}
	var nf *types.NotFound
	if errors.As(err, &nf) {
		return true
	}
	var apiErr smithy.APIError
	if errors.As(err, &apiErr) {
		switch apiErr.ErrorCode() {
		case "NoSuchKey", "NotFound", "404":
			return true
		}
	}
	var re *smithyhttp.ResponseError
	if errors.As(err, &re) && re.HTTPStatusCode() == http.StatusNotFound {
		return true
	}
	return false
}

func isPreconditionFailed(err error) bool {
	if err == nil {
		return false
	}
	var apiErr smithy.APIError
	if errors.As(err, &apiErr) {
		switch apiErr.ErrorCode() {
		case "PreconditionFailed", "ConditionalRequestConflict":
			return true
		}
	}
	var re *smithyhttp.ResponseError
	if errors.As(err, &re) && re.HTTPStatusCode() == http.StatusPreconditionFailed {
		return true
	}
	return false
}

func (b *Backend) headMeta(ctx context.Context, key string) (storage.ObjectMeta, error) {
	out, err := b.client.HeadObject(ctx, &s3.HeadObjectInput{
		Bucket: aws.String(b.bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		if isNotFound(err) {
			return storage.ObjectMeta{}, storage.ErrNotFound
		}
		return storage.ObjectMeta{}, fmt.Errorf("s3compat: head %q: %w", key, err)
	}
	return b.metaFromUserMetadata(key, trimETag(out.ETag), aws.ToInt64(out.ContentLength), out.Metadata, out.LastModified), nil
}

func (b *Backend) metaFromUserMetadata(key, revision string, size int64, userMeta map[string]string, lastModified *time.Time) storage.ObjectMeta {
	meta := storage.ObjectMeta{
		Path:     b.pathFromKey(key),
		Revision: revision,
		Size:     size,
	}
	if userMeta != nil {
		meta.UpdatedBy = userMeta[metaKeyUpdatedBy]
		if ts, ok := userMeta[metaKeyUpdatedAt]; ok {
			if parsed, err := time.Parse(time.RFC3339Nano, ts); err == nil {
				meta.UpdatedAt = parsed
			}
		}
	}
	if meta.UpdatedAt.IsZero() && lastModified != nil {
		meta.UpdatedAt = *lastModified
	}
	return meta
}

// Get implements storage.StorageBackend.
func (b *Backend) Get(ctx context.Context, objPath string) (io.ReadCloser, storage.ObjectMeta, error) {
	if err := ctx.Err(); err != nil {
		return nil, storage.ObjectMeta{}, err
	}
	key, err := b.resolveKey(objPath)
	if err != nil {
		return nil, storage.ObjectMeta{}, err
	}

	out, err := b.client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(b.bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		if isNotFound(err) {
			return nil, storage.ObjectMeta{}, storage.ErrNotFound
		}
		return nil, storage.ObjectMeta{}, fmt.Errorf("s3compat: get %q: %w", objPath, err)
	}

	meta := b.metaFromUserMetadata(key, trimETag(out.ETag), aws.ToInt64(out.ContentLength), out.Metadata, out.LastModified)
	return out.Body, meta, nil
}

// Put implements storage.StorageBackend.
func (b *Backend) Put(ctx context.Context, objPath string, r io.Reader, opts storage.PutOptions) (storage.ObjectMeta, error) {
	if err := ctx.Err(); err != nil {
		return storage.ObjectMeta{}, err
	}
	key, err := b.resolveKey(objPath)
	if err != nil {
		return storage.ObjectMeta{}, err
	}

	data, err := io.ReadAll(r)
	if err != nil {
		return storage.ObjectMeta{}, fmt.Errorf("s3compat: reading input for %q: %w", objPath, err)
	}

	now := b.clock.Now()
	put := &s3.PutObjectInput{
		Bucket:        aws.String(b.bucket),
		Key:           aws.String(key),
		Body:          bytes.NewReader(data),
		ContentLength: aws.Int64(int64(len(data))),
		Metadata: map[string]string{
			metaKeyUpdatedAt: now.UTC().Format(time.RFC3339Nano),
		},
	}
	if opts.UpdatedBy != "" {
		put.Metadata[metaKeyUpdatedBy] = opts.UpdatedBy
	}

	if opts.IfMatchRevision != "" {
		current, err := b.headMeta(ctx, key)
		if err != nil && !errors.Is(err, storage.ErrNotFound) {
			return storage.ObjectMeta{}, err
		}
		currentRevision := ""
		if err == nil {
			currentRevision = current.Revision
		}
		if currentRevision != opts.IfMatchRevision {
			return storage.ObjectMeta{}, storage.ErrRevisionMismatch
		}
		put.IfMatch = quoteETag(currentRevision)
	}

	out, err := b.client.PutObject(ctx, put)
	if err != nil {
		if isPreconditionFailed(err) {
			return storage.ObjectMeta{}, storage.ErrRevisionMismatch
		}
		return storage.ObjectMeta{}, fmt.Errorf("s3compat: put %q: %w", objPath, err)
	}

	meta := storage.ObjectMeta{
		Path:      objPath,
		Revision:  trimETag(out.ETag),
		Size:      int64(len(data)),
		UpdatedAt: now,
		UpdatedBy: opts.UpdatedBy,
	}

	if !b.versioningEnabled {
		if err := b.recordRevision(ctx, key, meta); err != nil {
			return storage.ObjectMeta{}, fmt.Errorf("s3compat: recording revision history for %q: %w", objPath, err)
		}
	}

	return meta, nil
}

// Delete implements storage.StorageBackend.
func (b *Backend) Delete(ctx context.Context, objPath string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	key, err := b.resolveKey(objPath)
	if err != nil {
		return err
	}

	if _, err := b.headMeta(ctx, key); err != nil {
		return err
	}

	if _, err := b.client.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: aws.String(b.bucket),
		Key:    aws.String(key),
	}); err != nil {
		return fmt.Errorf("s3compat: deleting %q: %w", objPath, err)
	}

	if !b.versioningEnabled {
		_, _ = b.client.DeleteObject(ctx, &s3.DeleteObjectInput{
			Bucket: aws.String(b.bucket),
			Key:    aws.String(b.sidecarKey(key)),
		})
	}

	return nil
}

// List implements storage.StorageBackend.
func (b *Backend) List(ctx context.Context, prefix string) ([]storage.ObjectMeta, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	fullPrefix := prefix
	if b.prefix != "" {
		if prefix == "" {
			fullPrefix = b.prefix + "/"
		} else {
			fullPrefix = b.prefix + "/" + prefix
		}
	}

	var results []storage.ObjectMeta
	var continuationToken *string
	for {
		out, err := b.client.ListObjectsV2(ctx, &s3.ListObjectsV2Input{
			Bucket:            aws.String(b.bucket),
			Prefix:            aws.String(fullPrefix),
			ContinuationToken: continuationToken,
		})
		if err != nil {
			return nil, fmt.Errorf("s3compat: listing prefix %q: %w", prefix, err)
		}

		for _, obj := range out.Contents {
			key := aws.ToString(obj.Key)
			if b.isUnderMetaDir(key) {
				continue
			}
			results = append(results, storage.ObjectMeta{
				Path:      b.pathFromKey(key),
				Revision:  trimETag(obj.ETag),
				Size:      aws.ToInt64(obj.Size),
				UpdatedAt: aws.ToTime(obj.LastModified),
			})
		}

		if !aws.ToBool(out.IsTruncated) {
			break
		}
		continuationToken = out.NextContinuationToken
	}

	sort.Slice(results, func(i, j int) bool { return results[i].Path < results[j].Path })
	return results, nil
}

// Stat implements storage.StorageBackend.
func (b *Backend) Stat(ctx context.Context, objPath string) (storage.ObjectMeta, error) {
	if err := ctx.Err(); err != nil {
		return storage.ObjectMeta{}, err
	}
	key, err := b.resolveKey(objPath)
	if err != nil {
		return storage.ObjectMeta{}, err
	}
	return b.headMeta(ctx, key)
}

// Revisions implements storage.StorageBackend.
func (b *Backend) Revisions(ctx context.Context, objPath string) ([]storage.ObjectMeta, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	key, err := b.resolveKey(objPath)
	if err != nil {
		return nil, err
	}

	current, err := b.headMeta(ctx, key)
	if err != nil {
		return nil, err
	}

	if b.versioningEnabled {
		return b.listObjectVersions(ctx, key, current)
	}
	return b.loadRevisionHistory(ctx, key), nil
}

func (b *Backend) listObjectVersions(ctx context.Context, key string, current storage.ObjectMeta) ([]storage.ObjectMeta, error) {
	out, err := b.client.ListObjectVersions(ctx, &s3.ListObjectVersionsInput{
		Bucket: aws.String(b.bucket),
		Prefix: aws.String(key),
	})
	if err != nil {
		return nil, fmt.Errorf("s3compat: listing versions for %q: %w", key, err)
	}

	var versions []types.ObjectVersion
	for _, v := range out.Versions {
		if aws.ToString(v.Key) == key {
			versions = append(versions, v)
		}
	}
	sort.Slice(versions, func(i, j int) bool {
		ti, tj := versions[i].LastModified, versions[j].LastModified
		if ti == nil || tj == nil {
			return false
		}
		return ti.Before(*tj)
	})

	result := make([]storage.ObjectMeta, 0, len(versions))
	for _, v := range versions {
		meta := storage.ObjectMeta{
			Path:      b.pathFromKey(key),
			Revision:  trimETag(v.ETag),
			Size:      aws.ToInt64(v.Size),
			UpdatedAt: aws.ToTime(v.LastModified),
		}
		if aws.ToBool(v.IsLatest) {
			meta.UpdatedBy = current.UpdatedBy
		}
		result = append(result, meta)
	}
	return result, nil
}

// Health implements storage.StorageBackend.
func (b *Backend) Health(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if _, err := b.client.HeadBucket(ctx, &s3.HeadBucketInput{Bucket: aws.String(b.bucket)}); err != nil {
		return fmt.Errorf("s3compat: bucket %q not reachable: %w", b.bucket, err)
	}
	return nil
}

// Close implements storage.StorageBackend.
func (b *Backend) Close() error {
	return nil
}

type revisionRecord struct {
	Meta storage.ObjectMeta `json:"meta"`
}

func (b *Backend) recordRevision(ctx context.Context, key string, meta storage.ObjectMeta) error {
	sidecarKey := b.sidecarKey(key)

	existing := b.loadRevisionRecords(ctx, sidecarKey)
	existing = append(existing, revisionRecord{Meta: meta})
	if len(existing) > maxRevisionHistory {
		existing = existing[len(existing)-maxRevisionHistory:]
	}

	data, err := json.Marshal(existing)
	if err != nil {
		return err
	}
	_, err = b.client.PutObject(ctx, &s3.PutObjectInput{
		Bucket:        aws.String(b.bucket),
		Key:           aws.String(sidecarKey),
		Body:          bytes.NewReader(data),
		ContentLength: aws.Int64(int64(len(data))),
	})
	return err
}

func (b *Backend) loadRevisionRecords(ctx context.Context, sidecarKey string) []revisionRecord {
	out, err := b.client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(b.bucket),
		Key:    aws.String(sidecarKey),
	})
	if err != nil {
		return nil
	}
	defer out.Body.Close()

	data, err := io.ReadAll(out.Body)
	if err != nil {
		return nil
	}
	var records []revisionRecord
	if err := json.Unmarshal(data, &records); err != nil {
		return nil
	}
	return records
}

func (b *Backend) loadRevisionHistory(ctx context.Context, key string) []storage.ObjectMeta {
	records := b.loadRevisionRecords(ctx, b.sidecarKey(key))
	result := make([]storage.ObjectMeta, 0, len(records))
	for _, r := range records {
		result = append(result, r.Meta)
	}
	return result
}
