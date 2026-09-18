package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/acumen-ai-org/robotdreams/pkg/reporting"
)

const (
	ReportInstanceRetention = 50
	ReportEventRetention    = 500
)

const reportTimeLayout = "2006-01-02T15:04:05.000000000Z"

func formatReportTime(t time.Time) string {
	return t.UTC().Format(reportTimeLayout)
}

func parseReportTime(s string) (time.Time, error) {
	return time.Parse(time.RFC3339Nano, s)
}

func (s *Store) InsertReportInstance(ctx context.Context, inst *reporting.Instance) error {
	if inst == nil || inst.Definition == "" || inst.Scope == "" {
		return fmt.Errorf("server/store: insert report instance: definition and scope are required")
	}
	payload, err := json.Marshal(inst)
	if err != nil {
		return fmt.Errorf("server/store: marshal report instance: %w", err)
	}

	_, err = s.db.ExecContext(ctx, `
		INSERT INTO report_instances (definition, scope, producer, produced_at, payload)
		VALUES (?, ?, ?, ?, ?)`,
		inst.Definition, inst.Scope, inst.Producer,
		formatReportTime(inst.ProducedAt), string(payload),
	)
	if err != nil {
		return fmt.Errorf("server/store: insert report instance: %w", err)
	}

	return s.pruneReportInstances(ctx, inst.Definition, inst.Scope)
}

func (s *Store) pruneReportInstances(ctx context.Context, definition, scope string) error {
	_, err := s.db.ExecContext(ctx, `
		DELETE FROM report_instances
		WHERE definition = ? AND scope = ? AND id NOT IN (
			SELECT id FROM report_instances
			WHERE definition = ? AND scope = ?
			ORDER BY produced_at DESC, id DESC
			LIMIT ?)`,
		definition, scope, definition, scope, ReportInstanceRetention,
	)
	if err != nil {
		return fmt.Errorf("server/store: prune report instances: %w", err)
	}
	return nil
}

type ReportInstanceWindow struct {
	From  time.Time
	Until time.Time
}

func (s *Store) ListReportInstances(ctx context.Context, definition, scopePrefix string, window ...ReportInstanceWindow) ([]*reporting.Instance, error) {
	query := `SELECT payload FROM report_instances`
	var conds []string
	var args []any
	if definition != "" {
		conds = append(conds, `definition = ?`)
		args = append(args, definition)
	}
	for _, w := range window {
		if !w.From.IsZero() {
			conds = append(conds, `produced_at >= ?`)
			args = append(args, formatReportTime(w.From))
		}
		if !w.Until.IsZero() {
			conds = append(conds, `produced_at < ?`)
			args = append(args, formatReportTime(w.Until))
		}
	}
	if len(conds) > 0 {
		query += ` WHERE ` + strings.Join(conds, ` AND `) //nolint:gosec // G202: fixed fragments, values in args
	}
	query += ` ORDER BY produced_at ASC, id ASC`

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("server/store: list report instances: %w", err)
	}
	defer rows.Close()

	out := make([]*reporting.Instance, 0)
	for rows.Next() {
		var payload string
		if err := rows.Scan(&payload); err != nil {
			return nil, fmt.Errorf("server/store: list report instances: %w", err)
		}
		var inst reporting.Instance
		if err := json.Unmarshal([]byte(payload), &inst); err != nil {
			return nil, fmt.Errorf("server/store: decode report instance payload: %w", err)
		}
		if !reporting.ScopeWithin(scopePrefix, inst.Scope) {
			continue
		}
		out = append(out, &inst)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("server/store: iterate report instance rows: %w", err)
	}
	return out, nil
}

func (s *Store) InsertReportEvents(ctx context.Context, definition, scope string, events []reporting.Event) error {
	if definition == "" || scope == "" {
		return fmt.Errorf("server/store: insert report events: definition and scope are required")
	}
	if len(events) == 0 {
		return nil
	}

	for _, e := range events {
		var attrs sql.NullString
		if e.Attrs != nil {
			b, err := json.Marshal(e.Attrs)
			if err != nil {
				return fmt.Errorf("server/store: marshal report event attrs: %w", err)
			}
			attrs = sql.NullString{String: string(b), Valid: true}
		}
		_, err := s.db.ExecContext(ctx, `
			INSERT INTO report_events (definition, scope, at, type, severity, label, attrs)
			VALUES (?, ?, ?, ?, ?, ?, ?)`,
			definition, scope, formatReportTime(e.T), e.Type, e.Severity, e.Label, attrs,
		)
		if err != nil {
			return fmt.Errorf("server/store: insert report event: %w", err)
		}
	}

	return s.pruneReportEvents(ctx, definition, scope)
}

func (s *Store) pruneReportEvents(ctx context.Context, definition, scope string) error {
	_, err := s.db.ExecContext(ctx, `
		DELETE FROM report_events
		WHERE definition = ? AND scope = ? AND id NOT IN (
			SELECT id FROM report_events
			WHERE definition = ? AND scope = ?
			ORDER BY at DESC, id DESC
			LIMIT ?)`,
		definition, scope, definition, scope, ReportEventRetention,
	)
	if err != nil {
		return fmt.Errorf("server/store: prune report events: %w", err)
	}
	return nil
}

type ReportEventFilter struct {
	Definitions []string
	ScopePrefix string
	Since       time.Time
	Until       time.Time
}

func (s *Store) ListReportEvents(ctx context.Context, f ReportEventFilter) ([]reporting.Event, error) {
	query := `SELECT definition, scope, at, type, severity, label, attrs FROM report_events`
	var conds []string
	var args []any
	if len(f.Definitions) > 0 {
		conds = append(conds, `definition IN (?`+strings.Repeat(", ?", len(f.Definitions)-1)+`)`)
		for _, d := range f.Definitions {
			args = append(args, d)
		}
	}
	if !f.Since.IsZero() {
		conds = append(conds, `at > ?`)
		args = append(args, formatReportTime(f.Since))
	}
	if !f.Until.IsZero() {
		conds = append(conds, `at < ?`)
		args = append(args, formatReportTime(f.Until))
	}
	if len(conds) > 0 {
		query += ` WHERE ` + strings.Join(conds, ` AND `) //nolint:gosec // G202: fixed fragments, values in args
	}
	query += ` ORDER BY at ASC, id ASC`

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("server/store: list report events: %w", err)
	}
	defer rows.Close()

	out := make([]reporting.Event, 0)
	for rows.Next() {
		var (
			definition, scope, at string
			typ, severity, label  string
			attrs                 sql.NullString
		)
		if err := rows.Scan(&definition, &scope, &at, &typ, &severity, &label, &attrs); err != nil {
			return nil, fmt.Errorf("server/store: list report events: %w", err)
		}
		if !reporting.ScopeWithin(f.ScopePrefix, scope) {
			continue
		}
		t, err := parseReportTime(at)
		if err != nil {
			return nil, fmt.Errorf("server/store: decode report event time %q: %w", at, err)
		}
		e := reporting.Event{
			T:          t,
			Type:       typ,
			Severity:   severity,
			Label:      label,
			Scope:      scope,
			Definition: definition,
		}
		if attrs.Valid {
			var m map[string]string
			if err := json.Unmarshal([]byte(attrs.String), &m); err != nil {
				return nil, fmt.Errorf("server/store: decode report event attrs: %w", err)
			}
			e.Attrs = m
		}
		out = append(out, e)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("server/store: iterate report event rows: %w", err)
	}
	return out, nil
}

type ReportScopeCount struct {
	Path      string
	Instances int
	Recent    int
}

func (s *Store) ListReportScopes(ctx context.Context, since time.Time) ([]ReportScopeCount, error) {
	bound := ""
	if !since.IsZero() {
		bound = formatReportTime(since)
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT scope, COUNT(*), COUNT(CASE WHEN ? != '' AND produced_at > ? THEN 1 END)
		FROM report_instances
		GROUP BY scope ORDER BY scope ASC`, bound, bound)
	if err != nil {
		return nil, fmt.Errorf("server/store: list report scopes: %w", err)
	}
	defer rows.Close()

	out := make([]ReportScopeCount, 0)
	for rows.Next() {
		var c ReportScopeCount
		if err := rows.Scan(&c.Path, &c.Instances, &c.Recent); err != nil {
			return nil, fmt.Errorf("server/store: list report scopes: %w", err)
		}
		out = append(out, c)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("server/store: iterate report scope rows: %w", err)
	}
	return out, nil
}
