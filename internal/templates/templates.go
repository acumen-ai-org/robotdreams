package templates

import (
	"errors"
	"fmt"
	"io/fs"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

var ErrNoTemplate = errors.New("templates: no such template")

type Template struct {
	Name    string
	Summary string
	Assumes []string
	Brief   string
}

type meta struct {
	Summary string   `yaml:"summary"`
	Assumes []string `yaml:"assumes"`
}

const (
	libraryDir = "library"
	metaFile   = "meta.yaml"
	briefFile  = "brief.md"
)

func List() []Template {
	entries, err := fs.ReadDir(libraryFS, libraryDir)
	if err != nil {
		panic("templates: embedded library is missing: " + err.Error())
	}

	var out []Template
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		t, err := load(e.Name())
		if err != nil {
			panic("templates: embedded template " + e.Name() + " is malformed: " + err.Error())
		}
		out = append(out, t)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

func Get(name string) (Template, error) {
	if name == "" {
		return Template{}, fmt.Errorf("%w: empty name (available: %s)", ErrNoTemplate, strings.Join(Names(), ", "))
	}
	t, err := load(name)
	if err != nil {
		return Template{}, fmt.Errorf("%w: %q (available: %s)", ErrNoTemplate, name, strings.Join(Names(), ", "))
	}
	return t, nil
}

func Names() []string {
	all := List()
	names := make([]string, 0, len(all))
	for _, t := range all {
		names = append(names, t.Name)
	}
	return names
}

func load(name string) (Template, error) {
	if name != strings.Trim(name, "./\\") || strings.ContainsAny(name, `/\`) {
		return Template{}, fmt.Errorf("invalid template name %q", name)
	}

	rawMeta, err := fs.ReadFile(libraryFS, libraryDir+"/"+name+"/"+metaFile)
	if err != nil {
		return Template{}, err
	}
	var m meta
	if err := yaml.Unmarshal(rawMeta, &m); err != nil {
		return Template{}, fmt.Errorf("parse %s: %w", metaFile, err)
	}
	if m.Summary == "" {
		return Template{}, fmt.Errorf("%s has no summary", metaFile)
	}

	brief, err := fs.ReadFile(libraryFS, libraryDir+"/"+name+"/"+briefFile)
	if err != nil {
		return Template{}, err
	}
	if len(strings.TrimSpace(string(brief))) == 0 {
		return Template{}, fmt.Errorf("%s is empty", briefFile)
	}

	return Template{
		Name:    name,
		Summary: m.Summary,
		Assumes: m.Assumes,
		Brief:   string(brief),
	}, nil
}
