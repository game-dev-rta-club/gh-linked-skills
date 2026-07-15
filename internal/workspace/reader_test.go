package workspace

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestReadSkillBuildsRawSnapshot(t *testing.T) {
	dir := t.TempDir()
	writeTestFile(t, filepath.Join(dir, "SKILL.md"), `---
name: sample
metadata:
  github-repo: https://github.com/owner/repo
  github-path: skills/sample
  github-ref: refs/heads/main
  github-tree-sha: base123
---
Body
`)
	writeTestFile(t, filepath.Join(dir, "scripts", "check.mjs"), "export default true;\n")

	got, err := ReadSkill(dir)

	if err != nil {
		t.Fatalf("ReadSkill() error = %v", err)
	}
	if string(got.Snapshot["SKILL.md"]) != string(got.Files["SKILL.md"]) {
		t.Fatal("Snapshot does not preserve raw SKILL.md")
	}
	if string(got.Snapshot["scripts/check.mjs"]) != "export default true;\n" {
		t.Fatalf("script content = %q", got.Snapshot["scripts/check.mjs"])
	}
	if string(got.Files["SKILL.md"]) == "" {
		t.Fatal("raw files missing SKILL.md")
	}
}

func TestReadSkillRejectsSymlink(t *testing.T) {
	dir := t.TempDir()
	writeTestFile(t, filepath.Join(dir, "SKILL.md"), "---\nname: sample\n---\nBody\n")
	if err := os.Symlink("SKILL.md", filepath.Join(dir, "linked.md")); err != nil {
		t.Fatal(err)
	}

	_, err := ReadSkill(dir)

	if !errors.Is(err, ErrUnsupportedFile) {
		t.Fatalf("ReadSkill() error = %v, want ErrUnsupportedFile", err)
	}
}

func TestReadSkillRecognizesGeneratedMarkersBeforeParsingFrontmatter(t *testing.T) {
	dir := t.TempDir()
	content := "---\n<<<<<<< gh-linked-skills:local\nname: local\n||||||| gh-linked-skills:base:base\nname: base\n=======\nname: remote\n>>>>>>> gh-linked-skills:remote:remote\n---\n"
	writeTestFile(t, filepath.Join(dir, "SKILL.md"), content)

	got, err := ReadSkill(dir)

	if err != nil {
		t.Fatalf("ReadSkill() error = %v, want conflict snapshot", err)
	}
	if !got.Snapshot.HasGeneratedConflictMarker() || string(got.Files["SKILL.md"]) != content {
		t.Fatalf("ReadSkill() = %#v, want raw generated conflict", got)
	}
}

func TestReadSkillRequiresSkillDocument(t *testing.T) {
	_, err := ReadSkill(t.TempDir())

	if !errors.Is(err, ErrInvalidSkill) {
		t.Fatalf("ReadSkill() error = %v, want ErrInvalidSkill", err)
	}
}

func writeTestFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
