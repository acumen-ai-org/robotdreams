package orgchart

import (
	"errors"
	"fmt"
	"os"
	"sort"

	"gopkg.in/yaml.v3"
)

type chartFile struct {
	Workers []chartWorker `yaml:"workers"`
}

type chartWorker struct {
	ID        string `yaml:"id"`
	Role      string `yaml:"role"`
	ReportsTo string `yaml:"reports_to"`
}

var ErrInvalidChartFile = errors.New("orgchart: invalid chart file")

func ParseChartFile(data []byte) ([]Worker, error) {
	var cf chartFile
	if err := yaml.Unmarshal(data, &cf); err != nil {
		return nil, fmt.Errorf("orgchart: parse chart file: %w", err)
	}

	byID, problems := indexUniqueIDs(cf.Workers)
	problems = append(problems, checkReportsToResolve(cf.Workers, byID)...)
	for _, id := range detectCycles(cf.Workers, byID) {
		problems = append(problems, fmt.Errorf("worker %q is part of a reports_to cycle", id))
	}

	if len(problems) > 0 {
		return nil, fmt.Errorf("%w: %w", ErrInvalidChartFile, errors.Join(problems...))
	}

	out := make([]Worker, 0, len(cf.Workers))
	for _, w := range cf.Workers {
		out = append(out, Worker{ID: w.ID, Role: w.Role, ReportsTo: w.ReportsTo})
	}
	return out, nil
}

func indexUniqueIDs(workers []chartWorker) (map[string]int, []error) {
	var problems []error
	byID := make(map[string]int, len(workers))
	for i, w := range workers {
		if w.ID == "" {
			problems = append(problems, fmt.Errorf("workers[%d]: empty id", i))
			continue
		}
		if prev, ok := byID[w.ID]; ok {
			problems = append(problems, fmt.Errorf("workers[%d]: duplicate id %q (first declared at workers[%d])", i, w.ID, prev))
			continue
		}
		byID[w.ID] = i
	}
	return byID, problems
}

func checkReportsToResolve(workers []chartWorker, byID map[string]int) []error {
	var problems []error
	for i, w := range workers {
		if w.ID == "" || w.ReportsTo == "" {
			continue
		}
		if w.ReportsTo == w.ID {
			problems = append(problems, fmt.Errorf("workers[%d]: worker %q reports to itself", i, w.ID))
			continue
		}
		if _, ok := byID[w.ReportsTo]; !ok {
			problems = append(problems, fmt.Errorf("workers[%d]: worker %q has dangling reports_to %q (no such id in file)", i, w.ID, w.ReportsTo))
		}
	}
	return problems
}

func detectCycles(workers []chartWorker, byID map[string]int) []string {
	parent := make(map[string]string, len(workers))
	for _, w := range workers {
		if w.ID == "" || w.ReportsTo == "" || w.ReportsTo == w.ID {
			continue
		}
		if _, ok := byID[w.ReportsTo]; !ok {
			continue
		}
		if _, dup := parent[w.ID]; dup {
			continue
		}
		parent[w.ID] = w.ReportsTo
	}

	const (
		visiting = 1
		done     = 2
	)
	state := make(map[string]int, len(parent))
	onCycle := make(map[string]bool)

	for id := range parent {
		if state[id] != 0 {
			continue
		}
		var path []string
		cur := id
		for {
			if s := state[cur]; s == done {
				break
			} else if s == visiting {
				markCycleFrom(path, cur, onCycle)
				break
			}
			state[cur] = visiting
			path = append(path, cur)
			next, ok := parent[cur]
			if !ok {
				break
			}
			cur = next
		}
		for _, p := range path {
			state[p] = done
		}
	}

	ids := make([]string, 0, len(onCycle))
	for id := range onCycle {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}

func markCycleFrom(path []string, cycleEntry string, onCycle map[string]bool) {
	start := 0
	for i, p := range path {
		if p == cycleEntry {
			start = i
			break
		}
	}
	for _, p := range path[start:] {
		onCycle[p] = true
	}
}

func LoadChartFile(path string) ([]Worker, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("orgchart: load chart file %q: %w", path, err)
	}
	workers, err := ParseChartFile(data)
	if err != nil {
		return nil, fmt.Errorf("orgchart: load chart file %q: %w", path, err)
	}
	return workers, nil
}
