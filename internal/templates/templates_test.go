package templates

import (
	"errors"
	"strings"
	"testing"
)

func TestEveryTemplateLoads(t *testing.T) {
	all := List()
	if len(all) == 0 {
		t.Fatal("the embedded library is empty")
	}
	for _, tpl := range all {
		t.Run(tpl.Name, func(t *testing.T) {
			if tpl.Summary == "" {
				t.Error("no summary; --list-templates would show a blank line")
			}
			if strings.TrimSpace(tpl.Brief) == "" {
				t.Error("empty brief")
			}
			if len(tpl.Assumes) == 0 {
				t.Error("no assumptions listed; a reader cannot tell whether the commands will work")
			}
			if strings.Contains(tpl.Brief, "{{") {
				t.Error("brief contains a Go template action, but briefs are printed verbatim")
			}
		})
	}
}

func TestGet(t *testing.T) {
	tpl, err := Get("linux-onstart-claude")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if tpl.Name != "linux-onstart-claude" {
		t.Fatalf("name = %q", tpl.Name)
	}
}

func TestGetUnknownNamesTheAlternatives(t *testing.T) {
	for _, name := range []string{"nope", "", "linux-onstart-CLAUDE"} {
		_, err := Get(name)
		if !errors.Is(err, ErrNoTemplate) {
			t.Fatalf("Get(%q) error = %v, want ErrNoTemplate", name, err)
		}
		if !strings.Contains(err.Error(), "linux-onstart-claude") {
			t.Fatalf("Get(%q) error does not list what exists: %v", name, err)
		}
	}
}

func TestGetRejectsPathTraversal(t *testing.T) {
	for _, name := range []string{"../embed.go", "..", "./linux-onstart-claude", "library/linux-onstart-claude"} {
		if _, err := Get(name); !errors.Is(err, ErrNoTemplate) {
			t.Fatalf("Get(%q) = %v, want it refused", name, err)
		}
	}
}

func TestNamesAreSorted(t *testing.T) {
	names := Names()
	for i := 1; i < len(names); i++ {
		if names[i-1] > names[i] {
			t.Fatalf("Names() is not sorted: %v", names)
		}
	}
}
