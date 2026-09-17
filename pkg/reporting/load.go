package reporting

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// LoadDefinitionFile reads and parses the ReportDefinition at path,
// applying the same validation as ParseDefinition.
func LoadDefinitionFile(path string) (*Definition, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reporting: load definition %q: %w", path, err)
	}
	d, err := ParseDefinition(data)
	if err != nil {
		return nil, fmt.Errorf("reporting: load definition %q: %w", path, err)
	}
	return d, nil
}

// LoadLibraryDir loads every *.yaml ReportDefinition in dir, in filename
// order, and validates the set as a whole.
//
// Beyond per-file validation it resolves extends chains (a child starts
// from a deep copy of its parent and any non-zero child field — each
// top-level field, and each facet slot individually — overrides
// wholesale), rejects missing extends targets and extends cycles, rejects
// duplicate definition names, and cross-validates detail drilldown links:
// every drilldown name must resolve to a definition in the loaded set.
// All problems across all files are collected and returned together in a
// single error wrapping ErrInvalidDefinition.
func LoadLibraryDir(dir string) ([]*Definition, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("reporting: load library %q: %w", dir, err)
	}

	type loaded struct {
		file string
		def  *Definition
	}
	var files []loaded
	var problems []error

	names := make([]string, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".yaml") {
			continue
		}
		names = append(names, e.Name())
	}
	sort.Strings(names)

	for _, name := range names {
		data, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			problems = append(problems, fmt.Errorf("%s: %w", name, err))
			continue
		}
		var d Definition
		if err := yaml.Unmarshal(data, &d); err != nil {
			problems = append(problems, fmt.Errorf("%s: parse: %w", name, err))
			continue
		}
		for _, p := range d.validateCore() {
			problems = append(problems, fmt.Errorf("%s: %w", name, p))
		}
		files = append(files, loaded{file: name, def: &d})
	}

	// Index by definition name; duplicates are errors.
	byName := make(map[string]*loaded, len(files))
	for i := range files {
		f := &files[i]
		if f.def.Name == "" {
			continue // already reported by validateCore
		}
		if prev, ok := byName[f.def.Name]; ok {
			problems = append(problems, fmt.Errorf("%s: duplicate definition name %q (first declared in %s)", f.file, f.def.Name, prev.file))
			continue
		}
		byName[f.def.Name] = f
	}

	// Resolve extends chains, memoized, with cycle detection.
	resolved := make(map[string]*Definition, len(files))
	const (
		resolving = 1
		done      = 2
	)
	state := make(map[string]int, len(files))
	var resolve func(f *loaded) (*Definition, error)
	resolve = func(f *loaded) (*Definition, error) {
		name := f.def.Name
		if d, ok := resolved[name]; ok {
			return d, nil
		}
		if state[name] == resolving {
			return nil, fmt.Errorf("%s: definition %q is part of an extends cycle", f.file, name)
		}
		state[name] = resolving
		defer func() { state[name] = done }()

		d := f.def
		if d.Extends != "" {
			if d.Extends == name {
				return nil, fmt.Errorf("%s: definition %q extends itself", f.file, name)
			}
			parentFile, ok := byName[d.Extends]
			if !ok {
				return nil, fmt.Errorf("%s: extends %q: no such definition in the loaded set", f.file, d.Extends)
			}
			parent, err := resolve(parentFile)
			if err != nil {
				return nil, err
			}
			d = mergeExtends(parent, d)
		}
		resolved[name] = d
		return d, nil
	}

	defs := make([]*Definition, 0, len(files))
	defFiles := make([]string, 0, len(files))
	for i := range files {
		f := &files[i]
		if f.def.Name == "" || byName[f.def.Name] != f {
			continue // unnamed or duplicate; already reported
		}
		d, err := resolve(f)
		if err != nil {
			problems = append(problems, err)
			continue
		}
		for _, p := range d.validateRefs() {
			problems = append(problems, fmt.Errorf("%s: %w", f.file, p))
		}
		defs = append(defs, d)
		defFiles = append(defFiles, f.file)
	}

	// Cross-file drilldown links must resolve within the loaded set.
	for i, d := range defs {
		if d.Facets.Detail == nil {
			continue
		}
		for j, target := range d.Facets.Detail.Drilldown {
			if _, ok := byName[target]; !ok {
				problems = append(problems, fmt.Errorf("%s: facets.detail.drilldown[%d]: unknown definition %q", defFiles[i], j, target))
			}
		}
	}

	if len(problems) > 0 {
		return nil, fmt.Errorf("reporting: load library %q: %w: %w", dir, ErrInvalidDefinition, errors.Join(problems...))
	}
	return defs, nil
}

// mergeExtends applies the inheritance rule: the child starts from a deep
// copy of the parent, and any non-zero child field overrides wholesale —
// no per-element merging. "Field" means each top-level Definition field
// (description, categories, modalities, scope, data), and each facet slot
// individually: a child facet block replaces the parent's block entirely.
func mergeExtends(parent, child *Definition) *Definition {
	out := parent.clone()
	out.Version = child.Version
	out.Kind = child.Kind
	out.Name = child.Name
	out.Extends = child.Extends
	if child.Description != "" {
		out.Description = child.Description
	}
	if len(child.Categories) > 0 {
		out.Categories = child.Categories
	}
	if len(child.Modalities) > 0 {
		out.Modalities = child.Modalities
	}
	if len(child.Stances) > 0 {
		out.Stances = child.Stances
	}
	if !child.Scope.isZero() {
		out.Scope = child.Scope
	}
	if !child.Data.isZero() {
		out.Data = child.Data
	}
	if child.Facets.Summary != nil {
		out.Facets.Summary = child.Facets.Summary
	}
	if child.Facets.Timeline != nil {
		out.Facets.Timeline = child.Facets.Timeline
	}
	if child.Facets.Detail != nil {
		out.Facets.Detail = child.Facets.Detail
	}
	if child.Facets.Plan != nil {
		out.Facets.Plan = child.Facets.Plan
	}
	if child.Facets.Pulse != nil {
		out.Facets.Pulse = child.Facets.Pulse
	}
	if child.Facets.Media != nil {
		out.Facets.Media = child.Facets.Media
	}
	if child.Facets.Conversation != nil {
		out.Facets.Conversation = child.Facets.Conversation
	}
	return out
}

func (s ScopeSpec) isZero() bool {
	a := s.Aggregation
	return len(s.Attach) == 0 && a.Status == "" && a.Headline == "" &&
		a.Timeline == "" && a.Pulse == "" && a.Items == "" && len(a.KPIs) == 0 && a.Time == nil
}

func (d DataContract) isZero() bool {
	return len(d.KPIs) == 0 && len(d.Series) == 0 && len(d.Events) == 0 && len(d.Tables) == 0
}

// clone returns a deep copy of the definition, so a child produced by
// mergeExtends never aliases its parent's slices or maps.
func (d *Definition) clone() *Definition {
	out := *d
	out.Categories = append([]string(nil), d.Categories...)
	out.Modalities = append([]string(nil), d.Modalities...)
	out.Stances = append([]string(nil), d.Stances...)
	out.Scope.Attach = append([]string(nil), d.Scope.Attach...)
	if d.Scope.Aggregation.KPIs != nil {
		kpis := make(map[string]string, len(d.Scope.Aggregation.KPIs))
		for k, v := range d.Scope.Aggregation.KPIs {
			kpis[k] = v
		}
		out.Scope.Aggregation.KPIs = kpis
	}
	if tm := d.Scope.Aggregation.Time; tm != nil {
		cp := *tm
		if tm.KPIs != nil {
			cp.KPIs = make(map[string]string, len(tm.KPIs))
			for k, v := range tm.KPIs {
				cp.KPIs[k] = v
			}
		}
		out.Scope.Aggregation.Time = &cp
	}
	out.Data.KPIs = append([]KPISpec(nil), d.Data.KPIs...)
	out.Data.Series = append([]SeriesSpec(nil), d.Data.Series...)
	out.Data.Events = append([]EventSpec(nil), d.Data.Events...)
	out.Data.Tables = make([]TableSpec, len(d.Data.Tables))
	for i, t := range d.Data.Tables {
		t.Columns = append([]string(nil), t.Columns...)
		out.Data.Tables[i] = t
	}
	if s := d.Facets.Summary; s != nil {
		cp := *s
		cp.KPIs = append([]string(nil), s.KPIs...)
		if s.Status != nil {
			st := *s.Status
			cp.Status = &st
		}
		out.Facets.Summary = &cp
	}
	if t := d.Facets.Timeline; t != nil {
		cp := *t
		cp.Events = append([]string(nil), t.Events...)
		cp.Spans = append([]SpanSpec(nil), t.Spans...)
		cp.Milestones = append([]string(nil), t.Milestones...)
		out.Facets.Timeline = &cp
	}
	if det := d.Facets.Detail; det != nil {
		cp := *det
		cp.Panels = append([]PanelSpec(nil), det.Panels...)
		cp.Drilldown = append([]string(nil), det.Drilldown...)
		out.Facets.Detail = &cp
	}
	if pf := d.Facets.Plan; pf != nil {
		cp := *pf
		cp.States = append([]string(nil), pf.States...)
		cp.Lanes = append([]string(nil), pf.Lanes...)
		cp.Horizons = append([]string(nil), pf.Horizons...)
		cp.Commitments = append([]string(nil), pf.Commitments...)
		cp.Levels = append([]string(nil), pf.Levels...)
		if pf.WIPLimits != nil {
			limits := make(map[string]int, len(pf.WIPLimits))
			for k, v := range pf.WIPLimits {
				limits[k] = v
			}
			cp.WIPLimits = limits
		}
		if pf.Baseline != nil {
			b := *pf.Baseline
			cp.Baseline = &b
		}
		out.Facets.Plan = &cp
	}
	if p := d.Facets.Pulse; p != nil {
		cp := *p
		cp.Events = append([]string(nil), p.Events...)
		out.Facets.Pulse = &cp
	}
	if m := d.Facets.Media; m != nil {
		cp := *m
		cp.Archetypes = append([]string(nil), m.Archetypes...)
		out.Facets.Media = &cp
	}
	if c := d.Facets.Conversation; c != nil {
		cp := *c
		cp.Grounding = append([]string(nil), c.Grounding...)
		cp.Prompts = append([]string(nil), c.Prompts...)
		out.Facets.Conversation = &cp
	}
	return &out
}
