package discovery

import (
	"reflect"
	"strings"
	"testing"
)

func TestFromTreeMatchesGhSkillRepositoryLayouts(t *testing.T) {
	entries := []Entry{
		{Path: "skills/alpha", Type: "tree", SHA: "tree-alpha"},
		{Path: "skills/alpha/SKILL.md", Type: "blob", SHA: "doc-alpha"},
		{Path: "skills/team/beta", Type: "tree", SHA: "tree-beta"},
		{Path: "skills/team/beta/SKILL.md", Type: "blob", SHA: "doc-beta"},
		{Path: "packages/tool/skills/gamma", Type: "tree", SHA: "tree-gamma"},
		{Path: "packages/tool/skills/gamma/SKILL.md", Type: "blob", SHA: "doc-gamma"},
		{Path: "plugins/writer/skills/delta", Type: "tree", SHA: "tree-delta"},
		{Path: "plugins/writer/skills/delta/SKILL.md", Type: "blob", SHA: "doc-delta"},
		{Path: "epsilon", Type: "tree", SHA: "tree-epsilon"},
		{Path: "epsilon/SKILL.md", Type: "blob", SHA: "doc-epsilon"},
		{Path: "SKILL.md", Type: "blob", SHA: "repository-root"},
		{Path: ".agents/skills/hidden", Type: "tree", SHA: "tree-hidden"},
		{Path: ".agents/skills/hidden/SKILL.md", Type: "blob", SHA: "doc-hidden"},
		{Path: "nested/plugins/writer/skills/not-supported", Type: "tree", SHA: "tree-nested-plugin"},
		{Path: "nested/plugins/writer/skills/not-supported/SKILL.md", Type: "blob", SHA: "doc-nested-plugin"},
	}

	result, err := FromTree("commit-sha", entries)
	if err != nil {
		t.Fatalf("FromTree() error = %v", err)
	}
	if result.CommitSHA != "commit-sha" {
		t.Fatalf("commit SHA = %q", result.CommitSHA)
	}
	got := make([]Skill, len(result.Skills))
	copy(got, result.Skills)
	want := []Skill{
		{Name: "alpha", Path: "skills/alpha", TreeSHA: "tree-alpha"},
		{Name: "epsilon", Path: "epsilon", TreeSHA: "tree-epsilon"},
		{Name: "gamma", Path: "packages/tool/skills/gamma", TreeSHA: "tree-gamma"},
		{Name: "beta", Namespace: "team", Path: "skills/team/beta", TreeSHA: "tree-beta"},
		{Name: "delta", Namespace: "writer", Path: "plugins/writer/skills/delta", TreeSHA: "tree-delta"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("skills = %#v, want %#v", got, want)
	}
}

func TestFromTreeRejectsMissingCommitAndMissingSkillTree(t *testing.T) {
	if _, err := FromTree("", nil); err == nil {
		t.Fatal("FromTree() missing commit error = nil")
	}
	_, err := FromTree("commit", []Entry{{Path: "skills/sample/SKILL.md", Type: "blob", SHA: "doc"}})
	if err == nil || !strings.Contains(err.Error(), "tree") {
		t.Fatalf("FromTree() error = %v, want missing tree", err)
	}
}

func TestResolveSelectsDisplayNameAndRejectsAmbiguousSimpleName(t *testing.T) {
	skills := []Skill{
		{Name: "review", Namespace: "alice", Path: "skills/alice/review"},
		{Name: "review", Namespace: "bob", Path: "skills/bob/review"},
		{Name: "ship", Path: "skills/ship"},
	}

	selected, err := Resolve(skills, "alice/review")
	if err != nil || selected.Path != "skills/alice/review" {
		t.Fatalf("Resolve(namespaced) = %#v, %v", selected, err)
	}
	selected, err = Resolve(skills, "ship")
	if err != nil || selected.Path != "skills/ship" {
		t.Fatalf("Resolve(simple) = %#v, %v", selected, err)
	}
	if _, err := Resolve(skills, "review"); err == nil || !strings.Contains(err.Error(), "ambiguous") {
		t.Fatalf("Resolve(ambiguous) error = %v", err)
	}
	if _, err := Resolve(skills, "missing"); err == nil || !strings.Contains(err.Error(), "not found") {
		t.Fatalf("Resolve(missing) error = %v", err)
	}
}

func TestFindNameCollisionsUsesFlatInstallName(t *testing.T) {
	err := FindNameCollisions([]Skill{
		{Name: "review", Namespace: "alice", Path: "skills/alice/review"},
		{Name: "review", Namespace: "bob", Path: "skills/bob/review"},
	})
	if err == nil || !strings.Contains(err.Error(), "alice/review") || !strings.Contains(err.Error(), "bob/review") {
		t.Fatalf("FindNameCollisions() error = %v", err)
	}
}

func TestLooksLikePathMatchesGhSkillSelectors(t *testing.T) {
	tests := map[string]bool{
		"review":                       false,
		"alice/review":                 false,
		"skills/review":                true,
		"skills/review/SKILL.md":       true,
		"packages/tool/skills/review":  true,
		"custom/location/review":       true,
		"plugins/writer/skills/review": true,
	}
	for selector, want := range tests {
		if got := LooksLikePath(selector); got != want {
			t.Errorf("LooksLikePath(%q) = %v, want %v", selector, got, want)
		}
	}
}
