package server

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/acumen-ai-org/robotdreams/internal/identity"
	"github.com/acumen-ai-org/robotdreams/internal/orgchart"
	"github.com/acumen-ai-org/robotdreams/internal/server/store"
	"github.com/acumen-ai-org/robotdreams/pkg/messaging"
	"github.com/acumen-ai-org/robotdreams/pkg/reporting"
	"github.com/acumen-ai-org/robotdreams/pkg/scheduling"
	"github.com/acumen-ai-org/robotdreams/pkg/security"
	"github.com/acumen-ai-org/robotdreams/pkg/storage"
	"github.com/acumen-ai-org/robotdreams/pkg/updates"
)

const (
	DefaultAddr = ":7420"

	DefaultKeyID = "server-1"

	ServerKeyFileName = "server.key"

	EnrollmentTokenFileName = "enrollment.token"

	DefaultTokenTTL = 15 * time.Minute

	DefaultDelegatedTokenTTL = 5 * time.Minute

	ControlWorkerID = "server"

	SubjectWorkerReassigned = "worker reassigned"

	SubjectUpdateAvailable = updates.SubjectUpdateAvailable
)

var DefaultScopes = []string{
	"message:send",
	"message:receive",
}

const AdminScope = "admin"

const EnrollScope = "enroll"

const MaxWorkerIDLen = 128

var workerIDRe = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*$`)

var ErrInvalidWorkerID = errors.New("server: invalid worker id")

func ValidateWorkerID(id string) error {
	switch {
	case id == "":
		return fmt.Errorf("worker_id is required: %w", ErrInvalidWorkerID)
	case len(id) > MaxWorkerIDLen:
		return fmt.Errorf("worker_id exceeds %d characters: %w", MaxWorkerIDLen, ErrInvalidWorkerID)
	case strings.Contains(id, DelegationSeparator):
		return fmt.Errorf("worker_id must not contain %s: %w", DelegationSeparator, ErrInvalidWorkerID)
	case !workerIDRe.MatchString(id):
		return fmt.Errorf("worker_id must start with a letter or digit and contain only letters, digits, '.', '-' and '_': %w", ErrInvalidWorkerID)
	}
	return nil
}

func storageOwnPrefix(workerID string) string {
	return "workers/" + workerID + "/"
}

const SharedStoragePrefix = "shared/"

func WorkerScopes(workerID string) []string {
	own := storageOwnPrefix(workerID)
	return append(append([]string(nil), DefaultScopes...),
		"storage:read:"+own+"*",
		"storage:write:"+own+"*",
		"storage:read:"+SharedStoragePrefix+"*",
		"storage:write:"+SharedStoragePrefix+"*",
	)
}

var ErrNoEscalationTarget = errors.New("server: worker reports to root, nothing to escalate to")

var ErrNotAuthorized = errors.New("server: caller not authorized")

var ErrInvalidMessage = errors.New("server: invalid message")

type Config struct {
	DataDir string

	MessagingURI string

	StorageURI string

	Addr string

	OrgChartFile string

	ReportsDir string

	TokenTTL time.Duration

	DelegatedTokenTTL time.Duration

	KeyID string

	Clock security.Clock

	OpenEnrollment bool
}

type Server struct {
	cfg       Config
	clock     security.Clock
	store     *store.Store
	graph     *orgchart.Graph
	issuer    *identity.Issuer
	messaging messaging.MessagingBackend
	storage   storage.StorageBackend
	reports   *reporting.Registry

	messagingURI        string
	storageURI          string
	enrollmentTokenPath string
}

func New(cfg Config) (*Server, error) {
	if cfg.DataDir == "" {
		return nil, fmt.Errorf("server: config: DataDir is required")
	}
	if cfg.Clock == nil {
		cfg.Clock = security.RealClock{}
	}
	if cfg.Addr == "" {
		cfg.Addr = DefaultAddr
	}
	if cfg.KeyID == "" {
		cfg.KeyID = DefaultKeyID
	}
	if cfg.TokenTTL <= 0 {
		cfg.TokenTTL = DefaultTokenTTL
	}
	if cfg.DelegatedTokenTTL <= 0 {
		cfg.DelegatedTokenTTL = DefaultDelegatedTokenTTL
	}

	dataDir, err := filepath.Abs(cfg.DataDir)
	if err != nil {
		return nil, fmt.Errorf("server: resolve data dir %q: %w", cfg.DataDir, err)
	}
	cfg.DataDir = dataDir
	if err := os.MkdirAll(dataDir, 0o700); err != nil {
		return nil, fmt.Errorf("server: create data dir %q: %w", dataDir, err)
	}

	st, err := store.Open(dataDir, cfg.Clock)
	if err != nil {
		return nil, err
	}

	s := &Server{
		cfg:   cfg,
		clock: cfg.Clock,
		store: st,
		graph: orgchart.NewGraph(),
	}

	ctx := context.Background()

	msgURI, stgURI, err := resolveBackendURIs(ctx, st, dataDir, cfg.MessagingURI, cfg.StorageURI)
	if err != nil {
		st.Close()
		return nil, err
	}
	if err := st.SetConfig(ctx, msgURI, stgURI); err != nil {
		st.Close()
		return nil, err
	}
	s.messagingURI, s.storageURI = msgURI, stgURI

	kp, err := identity.LoadOrGenerateKeyPair(filepath.Join(dataDir, ServerKeyFileName))
	if err != nil {
		st.Close()
		return nil, fmt.Errorf("server: load signing key: %w", err)
	}
	s.issuer = identity.NewIssuer(kp, cfg.KeyID, cfg.Clock)

	s.enrollmentTokenPath = EnrollmentTokenPath(dataDir)
	if _, err := loadOrGenerateEnrollmentToken(s.enrollmentTokenPath); err != nil {
		st.Close()
		return nil, fmt.Errorf("server: load enrollment token: %w", err)
	}

	mb, err := messaging.New(msgURI)
	if err != nil {
		st.Close()
		return nil, fmt.Errorf("server: open messaging backend %q: %w", msgURI, err)
	}
	s.messaging = mb

	sb, err := storage.New(stgURI)
	if err != nil {
		mb.Close()
		st.Close()
		return nil, fmt.Errorf("server: open storage backend %q: %w", stgURI, err)
	}
	s.storage = sb

	if err := s.loadReportLibrary(); err != nil {
		s.Close()
		return nil, err
	}

	if err := s.loadGraph(ctx); err != nil {
		s.Close()
		return nil, err
	}

	return s, nil
}

func resolveBackendURIs(ctx context.Context, st *store.Store, dataDir, cfgMsg, cfgStg string) (msgURI, stgURI string, err error) {
	msgURI, stgURI = cfgMsg, cfgStg

	if msgURI == "" || stgURI == "" {
		savedMsg, savedStg, gerr := st.GetConfig(ctx)
		switch {
		case gerr == nil:
			if msgURI == "" {
				msgURI = savedMsg
			}
			if stgURI == "" {
				stgURI = savedStg
			}
		case errors.Is(gerr, store.ErrNoConfig):
		default:
			return "", "", gerr
		}
	}

	if msgURI == "" {
		msgURI = DefaultMessagingURI(dataDir)
	}
	if stgURI == "" {
		stgURI = DefaultStorageURI(dataDir)
	}
	return msgURI, stgURI, nil
}

func EnrollmentTokenPath(dataDir string) string {
	return filepath.Join(dataDir, EnrollmentTokenFileName)
}

func loadOrGenerateEnrollmentToken(path string) (string, error) {
	tok, err := readEnrollmentToken(path)
	if err != nil {
		return "", err
	}
	if tok != "" {
		return tok, nil
	}

	return RotateEnrollmentToken(path)
}

func readEnrollmentToken(path string) (string, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", fmt.Errorf("server: read enrollment token %s: %w", path, err)
	}
	return strings.TrimSpace(string(raw)), nil
}

func RotateEnrollmentToken(path string) (string, error) {
	var b [32]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", fmt.Errorf("server: generate enrollment token: %w", err)
	}
	tok := hex.EncodeToString(b[:])

	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", fmt.Errorf("server: create enrollment token directory: %w", err)
	}
	tmp, err := os.CreateTemp(dir, "."+EnrollmentTokenFileName+".*")
	if err != nil {
		return "", fmt.Errorf("server: write enrollment token %s: %w", path, err)
	}
	tmpName := tmp.Name()
	cleanup := func() { _ = os.Remove(tmpName) }
	if err := tmp.Chmod(0o600); err != nil {
		tmp.Close()
		cleanup()
		return "", fmt.Errorf("server: write enrollment token %s: %w", path, err)
	}
	if _, err := tmp.WriteString(tok + "\n"); err != nil {
		tmp.Close()
		cleanup()
		return "", fmt.Errorf("server: write enrollment token %s: %w", path, err)
	}
	if err := tmp.Close(); err != nil {
		cleanup()
		return "", fmt.Errorf("server: write enrollment token %s: %w", path, err)
	}
	if err := os.Rename(tmpName, path); err != nil {
		cleanup()
		return "", fmt.Errorf("server: write enrollment token %s: %w", path, err)
	}
	return tok, nil
}

func DefaultMessagingURI(dataDir string) string {
	return "queue://sqlite" + filepath.ToSlash(filepath.Join(dataDir, "messages.db"))
}

func DefaultStorageURI(dataDir string) string {
	return "file://" + filepath.ToSlash(filepath.Join(dataDir, "storage"))
}

func (s *Server) loadGraph(ctx context.Context) error {
	workers, err := s.store.ListWorkers(ctx)
	if err != nil {
		return err
	}

	if len(workers) == 0 {
		return s.loadOrgChartFile(ctx)
	}

	pending := workers
	for len(pending) > 0 {
		var deferred []orgchart.Worker
		for _, w := range pending {
			if err := s.graph.AddWorker(w); err != nil {
				if errors.Is(err, orgchart.ErrParentNotFound) {
					deferred = append(deferred, w)
					continue
				}
				return fmt.Errorf("server: reload worker %q: %w", w.ID, err)
			}
		}
		if len(deferred) == len(pending) {
			return s.reattachOrphansToRoot(ctx, deferred)
		}
		pending = deferred
	}
	return nil
}

func (s *Server) reattachOrphansToRoot(ctx context.Context, orphans []orgchart.Worker) error {
	for _, w := range orphans {
		w.ReportsTo = ""
		if err := s.graph.AddWorker(w); err != nil {
			return fmt.Errorf("server: reload orphaned worker %q: %w", w.ID, err)
		}
		if err := s.store.UpsertWorker(ctx, w); err != nil {
			return err
		}
	}
	return nil
}

func (s *Server) loadOrgChartFile(ctx context.Context) error {
	if s.cfg.OrgChartFile == "" {
		return nil
	}

	declared, err := orgchart.LoadChartFile(s.cfg.OrgChartFile)
	if err != nil {
		return fmt.Errorf("server: load org chart: %w", err)
	}

	pending := declared
	for len(pending) > 0 {
		var deferred []orgchart.Worker
		progressed := false
		for _, w := range pending {
			if err := s.graph.AddWorker(w); err != nil {
				if errors.Is(err, orgchart.ErrParentNotFound) {
					deferred = append(deferred, w)
					continue
				}
				return fmt.Errorf("server: load org chart worker %q: %w", w.ID, err)
			}
			progressed = true
			if err := s.store.UpsertWorker(ctx, w); err != nil {
				return err
			}
		}
		if !progressed {
			return fmt.Errorf("server: load org chart: %d worker(s) have unresolvable reports_to edges", len(deferred))
		}
		pending = deferred
	}
	return nil
}

func (s *Server) loadReportLibrary() error {
	if s.cfg.ReportsDir == "" {
		s.reports = reporting.NewRegistry(nil)
		return nil
	}
	defs, err := reporting.LoadLibraryDir(s.cfg.ReportsDir)
	if err != nil {
		return fmt.Errorf("server: load report library: %w", err)
	}
	s.reports = reporting.NewRegistry(defs)
	return nil
}

func (s *Server) ConnectWorker(ctx context.Context, w orgchart.Worker) error {
	if err := ValidateWorkerID(w.ID); err != nil {
		return fmt.Errorf("%w: %w", orgchart.ErrInvalidWorker, err)
	}
	if w.Status == "" {
		w.Status = orgchart.StatusConnected
	}
	now := s.clock.Now()
	if w.ConnectedAt.IsZero() {
		w.ConnectedAt = now
	}
	if w.LastSeenAt.IsZero() {
		w.LastSeenAt = now
	}

	if err := s.graph.AddWorker(w); err != nil {
		return err
	}
	if err := s.store.UpsertWorker(ctx, w); err != nil {
		return err
	}
	return nil
}

func (s *Server) TouchWorker(ctx context.Context, workerID string) error {
	w, err := s.graph.Get(workerID)
	if err != nil {
		return err
	}
	w.LastSeenAt = s.clock.Now()
	w.Status = orgchart.StatusConnected
	return s.store.UpsertWorker(ctx, w)
}

func (s *Server) ReassignWorker(ctx context.Context, workerID, newParent, callerWorkerID string, callerIsAdmin bool) error {
	current, err := s.graph.Get(workerID)
	if err != nil {
		return err
	}

	oldParent := current.ReportsTo
	if !callerIsAdmin && callerWorkerID != workerID && callerWorkerID != oldParent {
		return fmt.Errorf("server: reassign %q by %q: caller is neither the worker, its current parent %q, nor an admin: %w",
			workerID, callerWorkerID, oldParent, ErrNotAuthorized)
	}

	if err := s.graph.Reassign(workerID, newParent); err != nil {
		return err
	}

	updated, err := s.graph.Get(workerID)
	if err != nil {
		return err
	}
	if err := s.store.UpsertWorker(ctx, updated); err != nil {
		return err
	}

	reassignedBy := callerWorkerID
	if reassignedBy == "" {
		reassignedBy = ControlWorkerID
	}
	body, err := json.Marshal(map[string]string{
		"worker_id":     workerID,
		"old_parent":    oldParent,
		"new_parent":    newParent,
		"reassigned_by": reassignedBy,
	})
	if err != nil {
		return fmt.Errorf("server: marshal reassignment notice: %w", err)
	}

	recipients := []string{workerID}
	if oldParent != "" {
		recipients = append(recipients, oldParent)
	}
	if newParent != "" && newParent != oldParent {
		recipients = append(recipients, newParent)
	}

	for _, to := range recipients {
		env := messaging.Envelope{
			ID:        NewID(),
			Type:      messaging.TypeStatusUpdate,
			From:      ControlWorkerID,
			To:        to,
			Subject:   SubjectWorkerReassigned,
			Body:      body,
			CreatedAt: s.clock.Now(),
		}
		if err := s.messaging.Emit(ctx, env); err != nil {
			return fmt.Errorf("server: announce reassignment to %q: %w", to, err)
		}
	}
	return nil
}

func (s *Server) EmitFromWorker(ctx context.Context, fromWorkerID string, env messaging.Envelope) (messaging.Envelope, error) {
	if fromWorkerID == "" {
		return messaging.Envelope{}, fmt.Errorf("server: emit: empty sender: %w", ErrInvalidMessage)
	}
	if _, err := s.graph.Get(fromWorkerID); err != nil {
		return messaging.Envelope{}, err
	}

	if env.StoragePtr != nil && env.StoragePtr.Path == "" {
		return messaging.Envelope{}, fmt.Errorf("server: emit: storage pointer has empty path: %w", ErrInvalidMessage)
	}

	env.From = fromWorkerID
	if env.ID == "" {
		env.ID = NewID()
	}
	if env.CreatedAt.IsZero() {
		env.CreatedAt = s.clock.Now()
	}

	parent, err := s.graph.EscalationTarget(fromWorkerID)
	if err != nil {
		return messaging.Envelope{}, err
	}

	switch env.Type {
	case messaging.TypeStatusUpdate, messaging.TypeEscalation:
		env.To = parent
	case messaging.TypeCompletedWork, messaging.TypeRequestForInput:
		if env.To != "" {
			if _, err := s.graph.Get(env.To); err != nil {
				return messaging.Envelope{}, fmt.Errorf("server: emit to %q: %w", env.To, err)
			}
		} else {
			env.To = parent
		}
	case messaging.TypeScheduled:
		if env.To == "" {
			return messaging.Envelope{}, fmt.Errorf("server: emit scheduled: no recipient: %w", ErrInvalidMessage)
		}
		if _, err := s.graph.Get(env.To); err != nil {
			return messaging.Envelope{}, fmt.Errorf("server: emit to %q: %w", env.To, err)
		}
	default:
		return messaging.Envelope{}, fmt.Errorf("server: emit: unknown message type %q: %w", env.Type, ErrInvalidMessage)
	}

	if env.To == "" {
		switch env.Type {
		case messaging.TypeEscalation, messaging.TypeRequestForInput:
			return messaging.Envelope{}, fmt.Errorf("server: emit %s from %q: %w", env.Type, fromWorkerID, ErrNoEscalationTarget)
		default:
			return env, nil
		}
	}

	if err := s.messaging.Emit(ctx, env); err != nil {
		return messaging.Envelope{}, err
	}
	return env, nil
}

func (s *Server) MintWorkerToken(workerID string) (security.Token, error) {
	return s.issuer.Mint(workerID, WorkerScopes(workerID), s.cfg.TokenTTL)
}

func (s *Server) MintAdminToken(subject string, ttl time.Duration) (security.Token, error) {
	if ttl <= 0 {
		ttl = s.cfg.TokenTTL
	}
	scopes := append(append([]string(nil), DefaultScopes...), AdminScope, "storage:read:*", "storage:write:*")
	return s.issuer.Mint(subject, scopes, ttl)
}

func (s *Server) Graph() *orgchart.Graph { return s.graph }

func (s *Server) Messaging() messaging.MessagingBackend { return s.messaging }

func (s *Server) Schedules() scheduling.Store { return s.store.Schedules() }

func (s *Server) Storage() storage.StorageBackend { return s.storage }

func (s *Server) Reports() *reporting.Registry { return s.reports }

func (s *Server) ReportsDir() string { return s.cfg.ReportsDir }

func (s *Server) Issuer() *identity.Issuer { return s.issuer }

func (s *Server) EnrollmentToken() string {
	tok, err := readEnrollmentToken(s.enrollmentTokenPath)
	if err != nil {
		return ""
	}
	return tok
}

func (s *Server) EnrollmentTokenPath() string { return s.enrollmentTokenPath }

func (s *Server) RotateEnrollmentToken() (string, error) {
	return RotateEnrollmentToken(s.enrollmentTokenPath)
}

func (s *Server) OpenEnrollment() bool { return s.cfg.OpenEnrollment }

func (s *Server) Revocations() *identity.SQLiteRevocationStore {
	return s.store.Revocations()
}

func (s *Server) Store() *store.Store { return s.store }

func (s *Server) Clock() security.Clock { return s.clock }

func (s *Server) Addr() string { return s.cfg.Addr }

func (s *Server) TokenTTL() time.Duration { return s.cfg.TokenTTL }

func (s *Server) DelegatedTokenTTL() time.Duration { return s.cfg.DelegatedTokenTTL }

func (s *Server) BackendURIs() (messagingURI, storageURI string) {
	return s.messagingURI, s.storageURI
}

func (s *Server) Health(ctx context.Context) error {
	return errors.Join(
		s.store.Health(ctx),
		s.messaging.Health(ctx),
		s.storage.Health(ctx),
	)
}

func (s *Server) Close() error {
	var errs []error
	if s.messaging != nil {
		if err := s.messaging.Close(); err != nil {
			errs = append(errs, err)
		}
	}
	if s.storage != nil {
		if err := s.storage.Close(); err != nil {
			errs = append(errs, err)
		}
	}
	if s.store != nil {
		if err := s.store.Close(); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

func NewID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return fmt.Sprintf("id-%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(b[:])
}

func ValidateBackendURI(uri string) error {
	u, err := url.Parse(uri)
	if err != nil {
		return fmt.Errorf("server: invalid backend URI %q: %w", uri, err)
	}
	if u.Scheme == "" {
		return fmt.Errorf("server: backend URI %q has no scheme", uri)
	}
	return nil
}
