package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"

	"github.com/acumen-ai-org/robotdreams/internal/orgchart"
	"github.com/acumen-ai-org/robotdreams/internal/server/store"
	"github.com/acumen-ai-org/robotdreams/pkg/messaging"
	"github.com/acumen-ai-org/robotdreams/pkg/updates"
)

var ErrInvalidUpdate = errors.New("server: invalid update")

func (s *Server) AnnounceUpdate(ctx context.Context, ann updates.Announcement, announcedBy string) (updates.Announcement, int, error) {
	if err := updates.ValidateKind(ann.Kind); err != nil {
		return updates.Announcement{}, 0, fmt.Errorf("%w: %s", ErrInvalidUpdate, err)
	}
	if ann.Version == "" {
		return updates.Announcement{}, 0, fmt.Errorf("%w: empty version", ErrInvalidUpdate)
	}
	if err := updates.ValidateSeverity(ann.Severity); err != nil {
		return updates.Announcement{}, 0, fmt.Errorf("%w: %s", ErrInvalidUpdate, err)
	}

	ann.ID = NewID()

	ann.AnnouncedAt = s.clock.Now().UTC()
	ann.AnnouncedBy = announcedBy
	if ann.AnnouncedBy == "" {
		ann.AnnouncedBy = ControlWorkerID
	}

	workers := s.workersByID()

	if err := s.store.InsertUpdateAnnouncement(ctx, ann, len(workers)); err != nil {
		return updates.Announcement{}, 0, err
	}

	body, err := json.Marshal(ann)
	if err != nil {
		return updates.Announcement{}, 0, fmt.Errorf("server: marshal update announcement: %w", err)
	}

	var (
		delivered int
		errs      []error
	)
	for _, w := range workers {
		env := messaging.Envelope{
			ID:        NewID(),
			Type:      messaging.TypeStatusUpdate,
			From:      ControlWorkerID,
			To:        w.ID,
			Subject:   SubjectUpdateAvailable,
			Body:      body,
			CreatedAt: s.clock.Now(),
		}
		if err := s.messaging.Emit(ctx, env); err != nil {
			errs = append(errs, fmt.Errorf("announce %s to %q: %w", ann.Kind, w.ID, err))
			continue
		}
		delivered++
	}

	return ann, delivered, errors.Join(errs...)
}

func (s *Server) RecordUpdateState(ctx context.Context, workerID string, rep updates.Report) error {
	if workerID == "" {
		return fmt.Errorf("%w: empty worker ID", ErrInvalidUpdate)
	}
	if _, err := s.graph.Get(workerID); err != nil {
		return err
	}
	if err := updates.ValidateKind(rep.Kind); err != nil {
		return fmt.Errorf("%w: %s", ErrInvalidUpdate, err)
	}
	if err := updates.ValidateStatus(rep.Status); err != nil {
		return fmt.Errorf("%w: %s", ErrInvalidUpdate, err)
	}

	return s.store.UpsertWorkerUpdateState(ctx, store.WorkerUpdateState{
		WorkerID:       workerID,
		Kind:           rep.Kind,
		CurrentVersion: rep.CurrentVersion,
		AnnouncementID: rep.AnnouncementID,
		TargetVersion:  rep.TargetVersion,
		Status:         rep.Status,
		Detail:         rep.Detail,
		ReportedAt:     s.clock.Now(),
	})
}

func (s *Server) LatestUpdateAnnouncements(ctx context.Context, kind string, limit int) ([]updates.Announcement, error) {
	if kind != "" {
		if err := updates.ValidateKind(kind); err != nil {
			return nil, fmt.Errorf("%w: %s", ErrInvalidUpdate, err)
		}
	}
	return s.store.ListUpdateAnnouncements(ctx, kind, limit)
}

func (s *Server) UpdateRollout(ctx context.Context, kind, announcementID string) (updates.Rollout, error) {
	if err := updates.ValidateKind(kind); err != nil {
		return updates.Rollout{}, fmt.Errorf("%w: %s", ErrInvalidUpdate, err)
	}

	out := updates.Rollout{Kind: kind, Counts: map[string]int{}}

	if announcementID != "" {
		ann, err := s.store.GetUpdateAnnouncement(ctx, announcementID)
		if err != nil {
			return updates.Rollout{}, err
		}
		out.Announcement = &ann
	} else {
		recent, err := s.store.ListUpdateAnnouncements(ctx, kind, 1)
		if err != nil {
			return updates.Rollout{}, err
		}
		if len(recent) > 0 {
			out.Announcement = &recent[0]

			announcementID = recent[0].ID
		}
	}

	states, err := s.store.ListWorkerUpdateState(ctx, store.WorkerUpdateFilter{Kind: kind})
	if err != nil {
		return updates.Rollout{}, err
	}
	byWorker := make(map[string]store.WorkerUpdateState, len(states))
	for _, st := range states {
		byWorker[st.WorkerID] = st
	}

	workers := s.workersByID()

	out.Nodes = make([]updates.RolloutNode, 0, len(workers))
	for _, w := range workers {
		out.Nodes = append(out.Nodes, rolloutNodeFor(w, byWorker[w.ID], kind, announcementID))
	}
	for _, n := range out.Nodes {
		out.Counts[n.Status]++
	}
	return out, nil
}

func (s *Server) workersByID() []orgchart.Worker {
	workers := s.graph.List()
	sort.Slice(workers, func(i, j int) bool { return workers[i].ID < workers[j].ID })
	return workers
}

func rolloutNodeFor(w orgchart.Worker, st store.WorkerUpdateState, kind, announcementID string) updates.RolloutNode {
	n := updates.RolloutNode{
		WorkerID:     w.ID,
		WorkerStatus: string(w.Status),
		Kind:         kind,
		Status:       updates.StatusUnknown,
	}
	if st.WorkerID == "" {
		return n
	}

	n.CurrentVersion = st.CurrentVersion

	if announcementID != "" && st.AnnouncementID != announcementID {
		return n
	}

	n.AnnouncementID = st.AnnouncementID
	n.TargetVersion = st.TargetVersion
	n.Detail = st.Detail
	n.ReportedAt = st.ReportedAt
	if st.Status != "" {
		n.Status = st.Status
	}
	return n
}

func (s *Server) PendingUpdatesFor(ctx context.Context, workerID string) ([]updates.Pending, error) {
	if _, err := s.graph.Get(workerID); err != nil {
		return nil, err
	}

	states, err := s.store.ListWorkerUpdateState(ctx, store.WorkerUpdateFilter{WorkerID: workerID})
	if err != nil {
		return nil, err
	}
	byKind := make(map[string]store.WorkerUpdateState, len(states))
	for _, st := range states {
		byKind[st.Kind] = st
	}

	all, err := s.store.ListUpdateAnnouncements(ctx, "", 0)
	if err != nil {
		return nil, err
	}

	seen := map[string]bool{}
	var out []updates.Pending
	for _, ann := range all {
		if seen[ann.Kind] {
			continue
		}
		seen[ann.Kind] = true

		st := byKind[ann.Kind]
		resolved := st.AnnouncementID == ann.ID &&
			(st.Status == updates.StatusApplied || st.Status == updates.StatusDeclined)
		if resolved {
			continue
		}
		p := updates.Pending{Announcement: ann, CurrentVersion: st.CurrentVersion, Status: updates.StatusUnknown}
		if st.AnnouncementID == ann.ID && st.Status != "" {
			p.Status = st.Status
		}
		out = append(out, p)
	}
	return out, nil
}
