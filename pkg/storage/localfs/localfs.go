// Package localfs is the reference storage.StorageBackend on a local filesystem directory, registered as file:///<path>.
package localfs

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"github.com/acumen-ai-org/robotdreams/pkg/security"
	"github.com/acumen-ai-org/robotdreams/pkg/storage"
)

const maxRevisionHistory = 10

// Backend is a storage.StorageBackend rooted at a local directory, with revision history in a sibling .meta directory.
type Backend struct {
	root  string
	clock security.Clock

	mu sync.Mutex
}

// New creates a Backend rooted at root, creating the directory if needed.
func New(root string) (*Backend, error) {
	return NewWithClock(root, security.RealClock{})
}

// NewWithClock is New with the clock used for UpdatedAt timestamps.
func NewWithClock(root string, clock security.Clock) (*Backend, error) {
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return nil, fmt.Errorf("localfs: resolving root %q: %w", root, err)
	}
	if err := os.MkdirAll(absRoot, 0o750); err != nil {
		return nil, fmt.Errorf("localfs: creating root %q: %w", absRoot, err)
	}
	return &Backend{root: absRoot, clock: clock}, nil
}

func init() {
	storage.Register("file", func(u *url.URL) (storage.StorageBackend, error) {
		p := u.Path
		if p == "" {
			p = u.Opaque
		}
		if p == "" && u.Host != "" {
			p = u.Host
		}
		if p == "" {
			return nil, fmt.Errorf("localfs: file URI %q has no path", u.String())
		}
		return New(p)
	})
}

func (b *Backend) resolvePath(objPath string) (full, rel string, err error) {
	if objPath == "" {
		return "", "", fmt.Errorf("localfs: empty path")
	}
	if filepath.IsAbs(objPath) {
		return "", "", fmt.Errorf("localfs: invalid path %q: must not be absolute", objPath)
	}
	for _, seg := range strings.Split(filepath.ToSlash(objPath), "/") {
		if seg == ".." {
			return "", "", fmt.Errorf("localfs: invalid path %q: must not contain \"..\" segments", objPath)
		}
	}

	rel = filepath.ToSlash(filepath.Clean(filepath.FromSlash(objPath)))

	if isReservedMetaPath(rel) {
		return "", "", fmt.Errorf("localfs: invalid path %q: %q is a reserved name", objPath, metaDirName)
	}

	full = filepath.Join(b.root, rel)

	if !isUnderRoot(b.root, full) {
		return "", "", fmt.Errorf("localfs: invalid path %q: escapes root", objPath)
	}
	return full, rel, nil
}

func isUnderRoot(root, full string) bool {
	return full == root || strings.HasPrefix(full, root+string(filepath.Separator))
}

func contentHash(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// Get implements storage.StorageBackend.
func (b *Backend) Get(ctx context.Context, path string) (io.ReadCloser, storage.ObjectMeta, error) {
	if err := ctx.Err(); err != nil {
		return nil, storage.ObjectMeta{}, err
	}
	full, rel, err := b.resolvePath(path)
	if err != nil {
		return nil, storage.ObjectMeta{}, err
	}

	b.mu.Lock()
	defer b.mu.Unlock()

	meta, err := b.statLocked(rel, full)
	if err != nil {
		return nil, storage.ObjectMeta{}, err
	}
	data, err := os.ReadFile(full)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, storage.ObjectMeta{}, storage.ErrNotFound
		}
		return nil, storage.ObjectMeta{}, fmt.Errorf("localfs: reading %q: %w", path, err)
	}
	return io.NopCloser(bytes.NewReader(data)), meta, nil
}

// Put implements storage.StorageBackend.
func (b *Backend) Put(ctx context.Context, path string, r io.Reader, opts storage.PutOptions) (storage.ObjectMeta, error) {
	if err := ctx.Err(); err != nil {
		return storage.ObjectMeta{}, err
	}
	full, rel, err := b.resolvePath(path)
	if err != nil {
		return storage.ObjectMeta{}, err
	}

	data, err := io.ReadAll(r)
	if err != nil {
		return storage.ObjectMeta{}, fmt.Errorf("localfs: reading input for %q: %w", path, err)
	}

	b.mu.Lock()
	defer b.mu.Unlock()

	if opts.IfMatchRevision != "" {
		current, err := b.statLocked(rel, full)
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
	}

	if err := os.MkdirAll(filepath.Dir(full), 0o750); err != nil {
		return storage.ObjectMeta{}, fmt.Errorf("localfs: creating parent dir for %q: %w", path, err)
	}

	if err := atomicWrite(full, data); err != nil {
		return storage.ObjectMeta{}, fmt.Errorf("localfs: writing %q: %w", path, err)
	}

	meta := storage.ObjectMeta{
		Path:      rel,
		Revision:  contentHash(data),
		Size:      int64(len(data)),
		UpdatedAt: b.clock.Now(),
		UpdatedBy: opts.UpdatedBy,
	}

	if err := b.recordRevision(rel, meta); err != nil {
		return storage.ObjectMeta{}, fmt.Errorf("localfs: recording revision for %q: %w", path, err)
	}

	return meta, nil
}

func atomicWrite(dest string, data []byte) error {
	dir := filepath.Dir(dest)
	tmp, err := os.CreateTemp(dir, ".tmp-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)

	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpName, dest)
}

// Delete implements storage.StorageBackend.
func (b *Backend) Delete(ctx context.Context, path string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	full, rel, err := b.resolvePath(path)
	if err != nil {
		return err
	}

	b.mu.Lock()
	defer b.mu.Unlock()

	if _, err := os.Stat(full); err != nil {
		if os.IsNotExist(err) {
			return storage.ErrNotFound
		}
		return fmt.Errorf("localfs: stat %q: %w", path, err)
	}
	if err := os.Remove(full); err != nil {
		return fmt.Errorf("localfs: deleting %q: %w", path, err)
	}
	b.removeRevisionHistory(rel)
	return nil
}

// List implements storage.StorageBackend.
func (b *Backend) List(ctx context.Context, prefix string) ([]storage.ObjectMeta, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	b.mu.Lock()
	defer b.mu.Unlock()

	var results []storage.ObjectMeta
	err := filepath.Walk(b.root, func(p string, info os.FileInfo, err error) error {
		if err != nil {
			if os.IsNotExist(err) {
				return nil
			}
			return err
		}
		if info.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(b.root, p)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		if isReservedMetaPath(rel) {
			return nil
		}
		if !strings.HasPrefix(rel, prefix) {
			return nil
		}
		meta, err := b.statLocked(rel, p)
		if err != nil {
			if errors.Is(err, storage.ErrNotFound) {
				return nil
			}
			return err
		}
		results = append(results, meta)
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("localfs: listing prefix %q: %w", prefix, err)
	}

	sort.Slice(results, func(i, j int) bool { return results[i].Path < results[j].Path })
	return results, nil
}

// Stat implements storage.StorageBackend.
func (b *Backend) Stat(ctx context.Context, path string) (storage.ObjectMeta, error) {
	if err := ctx.Err(); err != nil {
		return storage.ObjectMeta{}, err
	}
	full, rel, err := b.resolvePath(path)
	if err != nil {
		return storage.ObjectMeta{}, err
	}

	b.mu.Lock()
	defer b.mu.Unlock()

	return b.statLocked(rel, full)
}

func (b *Backend) statLocked(path, full string) (storage.ObjectMeta, error) {
	info, err := os.Stat(full)
	if err != nil {
		if os.IsNotExist(err) {
			return storage.ObjectMeta{}, storage.ErrNotFound
		}
		return storage.ObjectMeta{}, fmt.Errorf("localfs: stat %q: %w", path, err)
	}

	if latest, ok := b.latestRevision(path); ok {
		latest.Size = info.Size()
		return latest, nil
	}

	data, err := os.ReadFile(full)
	if err != nil {
		return storage.ObjectMeta{}, fmt.Errorf("localfs: reading %q for hash: %w", path, err)
	}
	return storage.ObjectMeta{
		Path:      path,
		Revision:  contentHash(data),
		Size:      info.Size(),
		UpdatedAt: info.ModTime(),
	}, nil
}

// Revisions implements storage.StorageBackend.
func (b *Backend) Revisions(ctx context.Context, path string) ([]storage.ObjectMeta, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	full, rel, err := b.resolvePath(path)
	if err != nil {
		return nil, err
	}

	b.mu.Lock()
	defer b.mu.Unlock()

	if _, err := os.Stat(full); err != nil {
		if os.IsNotExist(err) {
			return nil, storage.ErrNotFound
		}
		return nil, fmt.Errorf("localfs: stat %q: %w", path, err)
	}

	return b.loadRevisionHistory(rel), nil
}

// Health implements storage.StorageBackend.
func (b *Backend) Health(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	info, err := os.Stat(b.root)
	if err != nil {
		return fmt.Errorf("localfs: root %q not accessible: %w", b.root, err)
	}
	if !info.IsDir() {
		return fmt.Errorf("localfs: root %q is not a directory", b.root)
	}

	probe, err := os.CreateTemp(b.root, ".health-*")
	if err != nil {
		return fmt.Errorf("localfs: root %q not writable: %w", b.root, err)
	}
	name := probe.Name()
	probe.Close()
	if err := os.Remove(name); err != nil {
		return fmt.Errorf("localfs: cleaning up health probe: %w", err)
	}
	return nil
}

// Close implements storage.StorageBackend.
func (b *Backend) Close() error {
	return nil
}

const metaDirName = ".meta"

type revisionRecord struct {
	Seq  int `json:"seq"`
	Meta storage.ObjectMeta
}

func isReservedMetaPath(rel string) bool {
	return rel == metaDirName || strings.HasPrefix(rel, metaDirName+"/")
}

func (b *Backend) metaDir(path string) string {
	h := sha256.Sum256([]byte(path))
	return filepath.Join(b.root, metaDirName, hex.EncodeToString(h[:]))
}

func (b *Backend) recordRevision(path string, meta storage.ObjectMeta) error {
	dir := b.metaDir(path)
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return err
	}

	existing := b.loadRevisionRecordsLocked(dir)
	nextSeq := 0
	if len(existing) > 0 {
		nextSeq = existing[len(existing)-1].Seq + 1
	}

	rec := revisionRecord{Seq: nextSeq, Meta: meta}
	data, err := json.Marshal(rec)
	if err != nil {
		return err
	}
	fname := filepath.Join(dir, fmt.Sprintf("%08d.json", nextSeq))
	if err := atomicWrite(fname, data); err != nil {
		return err
	}

	existing = append(existing, rec)
	if len(existing) > maxRevisionHistory {
		toRemove := existing[:len(existing)-maxRevisionHistory]
		for _, r := range toRemove {
			os.Remove(filepath.Join(dir, fmt.Sprintf("%08d.json", r.Seq)))
		}
	}
	return nil
}

func (b *Backend) loadRevisionRecordsLocked(dir string) []revisionRecord {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var records []revisionRecord
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			continue
		}
		var rec revisionRecord
		if err := json.Unmarshal(data, &rec); err != nil {
			continue
		}
		records = append(records, rec)
	}
	sort.Slice(records, func(i, j int) bool { return records[i].Seq < records[j].Seq })
	return records
}

func (b *Backend) latestRevision(path string) (storage.ObjectMeta, bool) {
	records := b.loadRevisionRecordsLocked(b.metaDir(path))
	if len(records) == 0 {
		return storage.ObjectMeta{}, false
	}
	return records[len(records)-1].Meta, true
}

func (b *Backend) loadRevisionHistory(path string) []storage.ObjectMeta {
	records := b.loadRevisionRecordsLocked(b.metaDir(path))
	result := make([]storage.ObjectMeta, 0, len(records))
	for _, r := range records {
		result = append(result, r.Meta)
	}
	return result
}

func (b *Backend) removeRevisionHistory(path string) {
	os.RemoveAll(b.metaDir(path))
}
