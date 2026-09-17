package skill

import (
	"regexp"
	"strings"
	"testing"
)

func TestSkillFrontmatter(t *testing.T) {
	if !strings.HasPrefix(Markdown, "---\n") {
		t.Fatal("SKILL.md must start with YAML frontmatter")
	}
	parts := strings.SplitN(Markdown, "\n---\n", 2)
	if len(parts) != 2 {
		t.Fatal("frontmatter not terminated")
	}
	fm := parts[0]
	name := regexp.MustCompile(`(?m)^name:\s*"?([^"\n]+)"?`).FindStringSubmatch(fm)
	if name == nil || name[1] != Name {
		t.Fatalf("frontmatter name = %v, want %q", name, Name)
	}
	desc := regexp.MustCompile(`(?m)^description:\s*"?([^\n]+?)"?$`).FindStringSubmatch(fm)
	if desc == nil || len(desc[1]) == 0 || len(desc[1]) > 1024 {
		t.Fatalf("description missing or over 1024 chars")
	}
	if n := strings.Count(Markdown, "\n"); n > 500 {
		t.Fatalf("SKILL.md has %d lines; spec recommends < 500", n)
	}
}
