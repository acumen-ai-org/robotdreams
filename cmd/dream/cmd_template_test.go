package main

import (
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/acumen-ai-org/robotdreams/internal/templates"
)

var dreamCommandRE = regexp.MustCompile(`(?m)\bdream ([a-z-]+)(?: ([a-z-]+))?`)

var flagRE = regexp.MustCompile(`--[a-z][a-z-]*`)

func TestTemplateBriefsOnlyPrintRealCommands(t *testing.T) {
	root := newRootCmd()

	for _, tpl := range templates.List() {
		t.Run(tpl.Name, func(t *testing.T) {
			seen := map[string]bool{}
			for _, m := range dreamCommandRE.FindAllStringSubmatch(tpl.Brief, -1) {
				path := []string{m[1]}
				if m[2] != "" {
					path = append(path, m[2])
				}
				key := strings.Join(path, " ")
				if seen[key] {
					continue
				}
				seen[key] = true

				cmd := findByPath(root, path)
				if cmd == nil && len(path) == 2 {
					if parent := findByPath(root, path[:1]); parent != nil {
						continue
					}
				}
				if cmd == nil {
					t.Errorf("brief tells the reader to run `dream %s`, which is not a command", key)
				}
			}
			if len(seen) == 0 {
				t.Error("brief names no dream commands at all; it cannot be a usable recipe")
			}
		})
	}
}

func TestTemplateBriefFlagsExist(t *testing.T) {
	root := newRootCmd()

	for _, tpl := range templates.List() {
		t.Run(tpl.Name, func(t *testing.T) {
			for _, line := range strings.Split(tpl.Brief, "\n") {
				m := dreamCommandRE.FindStringSubmatch(line)
				if m == nil {
					continue
				}
				path := []string{m[1]}
				if m[2] != "" {
					path = append(path, m[2])
				}
				cmd := findByPath(root, path)
				if cmd == nil && len(path) == 2 {
					cmd = findByPath(root, path[:1])
				}
				if cmd == nil {
					continue
				}
				for _, flag := range flagRE.FindAllString(line, -1) {
					name := strings.TrimPrefix(flag, "--")
					if cmd.Flags().Lookup(name) == nil && cmd.PersistentFlags().Lookup(name) == nil {
						t.Errorf("brief prints `%s` on `%s`, which has no such flag\n  line: %s",
							flag, cmd.CommandPath(), strings.TrimSpace(line))
					}
				}
			}
		})
	}
}

func findByPath(root *cobra.Command, path []string) *cobra.Command {
	cur := root
	for _, name := range path {
		var next *cobra.Command
		for _, c := range cur.Commands() {
			if c.Name() == name {
				next = c
				break
			}
			for _, alias := range c.Aliases {
				if alias == name {
					next = c
					break
				}
			}
		}
		if next == nil {
			return nil
		}
		cur = next
	}
	return cur
}

func TestListTemplatesMentionsEvery(t *testing.T) {
	var sb strings.Builder
	printTemplateList(&sb)
	got := sb.String()

	names := templates.Names()
	sort.Strings(names)
	for _, name := range names {
		if !strings.Contains(got, name) {
			t.Errorf("--list-templates output omits %q", name)
		}
	}
	if !strings.Contains(got, "install") {
		t.Error("the listing does not say that templates install nothing")
	}
}

func TestPrintTemplateBriefLeadsWithAssumptions(t *testing.T) {
	var sb strings.Builder
	if err := printTemplateBrief(&sb, "linux-onstart-claude"); err != nil {
		t.Fatalf("printTemplateBrief: %v", err)
	}
	got := sb.String()
	if !strings.HasPrefix(got, "This template assumes:") {
		t.Fatalf("brief does not lead with its assumptions:\n%s", got[:min(200, len(got))])
	}
	if !strings.Contains(got, "worker connect") {
		t.Error("assumptions do not mention the connect step the recipe depends on")
	}
}

func TestPrintTemplateBriefUnknown(t *testing.T) {
	var sb strings.Builder
	if err := printTemplateBrief(&sb, "nope"); err == nil {
		t.Fatal("printing an unknown template succeeded")
	}
}
