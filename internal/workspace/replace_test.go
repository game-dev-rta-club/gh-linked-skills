package workspace

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/game-dev-rta-club/gh-skill-linker/internal/source"
)

func TestReplaceExactAtomicallyPreservesBytesAndModes(t *testing.T) {
	parent := t.TempDir()
	target := filepath.Join(parent, "sample")
	writeTestFile(t, filepath.Join(target, "SKILL.md"), "---\nname: old\n---\nOld\n")
	writeTestFile(t, filepath.Join(target, "obsolete.txt"), "remove\n")
	expected, err := ReadSkill(target)
	if err != nil {
		t.Fatal(err)
	}
	remote := source.SkillSnapshot{
		TreeSHA: "new-tree",
		Files: map[string][]byte{
			"SKILL.md":         []byte("---\nname:  exact\n---\nNew\n"),
			"scripts/check.sh": []byte("#!/bin/sh\nexit 0\n"),
		},
		Executable: map[string]bool{"scripts/check.sh": true},
	}
	committed := false

	err = ReplaceExact(target, remote, expected, func() error { committed = true; return nil })

	if err != nil {
		t.Fatalf("ReplaceExact() error = %v", err)
	}
	if !committed {
		t.Fatal("ReplaceExact() did not commit metadata")
	}
	content, _ := os.ReadFile(filepath.Join(target, "SKILL.md"))
	if string(content) != string(remote.Files["SKILL.md"]) {
		t.Fatalf("SKILL.md = %q, want exact remote bytes", content)
	}
	if _, err := os.Stat(filepath.Join(target, "obsolete.txt")); !os.IsNotExist(err) {
		t.Fatalf("obsolete file remains: %v", err)
	}
	info, err := os.Stat(filepath.Join(target, "scripts/check.sh"))
	if err != nil || info.Mode().Perm()&0o111 == 0 {
		t.Fatalf("script mode=%v error=%v, want executable", info.Mode(), err)
	}
}

func TestReplaceExactRollsBackWhenWorkspaceChanged(t *testing.T) {
	parent := t.TempDir()
	target := filepath.Join(parent, "sample")
	writeTestFile(t, filepath.Join(target, "SKILL.md"), "original\n")
	expected, err := ReadSkill(target)
	if err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, filepath.Join(target, "SKILL.md"), "concurrent\n")
	remote := source.SkillSnapshot{TreeSHA: "new-tree", Files: map[string][]byte{"SKILL.md": []byte("remote\n")}}

	err = ReplaceExact(target, remote, expected, nil)

	if !errors.Is(err, ErrWorkspaceChanged) {
		t.Fatalf("ReplaceExact() error = %v, want ErrWorkspaceChanged", err)
	}
	content, _ := os.ReadFile(filepath.Join(target, "SKILL.md"))
	if string(content) != "concurrent\n" {
		t.Fatalf("concurrent content not restored: %q", content)
	}
}

func TestReplaceExactRollsBackWhenCommitFails(t *testing.T) {
	parent := t.TempDir()
	target := filepath.Join(parent, "sample")
	writeTestFile(t, filepath.Join(target, "SKILL.md"), "original\n")
	expected, err := ReadSkill(target)
	if err != nil {
		t.Fatal(err)
	}
	remote := source.SkillSnapshot{TreeSHA: "new-tree", Files: map[string][]byte{"SKILL.md": []byte("remote\n")}}

	err = ReplaceExact(target, remote, expected, func() error { return errors.New("manifest failed") })

	if err == nil || !strings.Contains(err.Error(), "manifest failed") {
		t.Fatalf("ReplaceExact() error = %v, want commit failure", err)
	}
	content, _ := os.ReadFile(filepath.Join(target, "SKILL.md"))
	if string(content) != "original\n" {
		t.Fatalf("original not restored: %q", content)
	}
}
