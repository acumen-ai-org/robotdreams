package simulation

import (
	"context"
	"fmt"

	"github.com/acumen-ai-org/robotdreams/pkg/updates"
)

const (
	simCLIVersion     = "0.4.1"
	simCLINextVersion = "0.4.2"

	minFleetForForcedOutcomes = 20
)

type updateSpread struct {
	applied  float64
	failed   float64
	declined float64
}

var defaultSpread = updateSpread{applied: 0.72, failed: 0.03, declined: 0.02}

func (s *Sim) declareFleetVersions(ctx context.Context) {
	for _, spec := range s.co.Workers() {
		w, ok := s.workers[spec.ID]
		if !ok {
			continue
		}
		err := w.postUpdateReport(ctx, map[string]string{
			"kind":            updates.KindCLI,
			"status":          updates.StatusCurrent,
			"current_version": simCLIVersion,
		})
		if err != nil {
			fmt.Fprintf(s.opts.Out, "warning: %s could not declare its version: %v\n", spec.ID, err)
			return
		}
	}
}

func (s *Sim) announceFleetUpdate(ctx context.Context) error {
	var out struct {
		Announcement updates.Announcement `json:"announcement"`
		Recipients   int                  `json:"recipients"`
		Delivered    int                  `json:"delivered"`
	}
	body := map[string]string{
		"kind":     updates.KindCLI,
		"version":  simCLINextVersion,
		"severity": updates.SeverityRecommended,
		"source":   "https://github.com/acumen-ai-org/robotdreams/releases/tag/v" + simCLINextVersion,
		"notes":    "Finish what is in flight, then update and restart.",
	}
	if err := s.api.postJSON(ctx, "/api/updates", s.adminToken, body, &out); err != nil {
		return fmt.Errorf("announce update: %w", err)
	}

	s.reportUpdateSpread(ctx, out.Announcement.ID, defaultSpread)

	fmt.Fprintf(s.opts.Out, "announced %s %s to %d nodes\n",
		updates.KindCLI, simCLINextVersion, out.Delivered)
	return nil
}

func (s *Sim) reportUpdateSpread(ctx context.Context, announcementID string, sp updateSpread) {
	specs := s.co.Workers()
	n := len(specs)
	if n == 0 {
		return
	}

	order := s.rng.Perm(n)

	applied := int(float64(n) * sp.applied)
	failed := int(float64(n) * sp.failed)
	declined := int(float64(n) * sp.declined)
	if failed == 0 && n > minFleetForForcedOutcomes {
		failed = 1
	}
	if declined == 0 && n > minFleetForForcedOutcomes {
		declined = 1
	}

	for i, idx := range order {
		spec := specs[idx]
		w, ok := s.workers[spec.ID]
		if !ok {
			continue
		}

		report := map[string]string{
			"kind":            updates.KindCLI,
			"announcement_id": announcementID,
			"target_version":  simCLINextVersion,
		}
		switch {
		case i < applied:
			report["status"] = updates.StatusApplied
			report["current_version"] = simCLINextVersion
		case i < applied+failed:
			report["status"] = updates.StatusFailed
			report["current_version"] = simCLIVersion
			report["detail"] = "could not replace the binary: permission denied"
		case i < applied+failed+declined:
			report["status"] = updates.StatusDeclined
			report["current_version"] = simCLIVersion
			report["detail"] = "pinned by policy until the next maintenance window"
		default:
			continue
		}

		if err := w.postUpdateReport(ctx, report); err != nil {
			fmt.Fprintf(s.opts.Out, "warning: %s could not report its update: %v\n", spec.ID, err)
			return
		}
	}
}
