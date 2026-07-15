package pull

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/game-dev-rta-club/gh-linked-skills/internal/command"
	"github.com/game-dev-rta-club/gh-linked-skills/internal/gitcli"
	"github.com/game-dev-rta-club/gh-linked-skills/internal/manifest"
	"github.com/game-dev-rta-club/gh-linked-skills/internal/source"
	"github.com/game-dev-rta-club/gh-linked-skills/internal/workspace"
)

const (
	baseCommit   = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	baseTree     = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	remoteCommit = "cccccccccccccccccccccccccccccccccccccccc"
	remoteTree   = "dddddddddddddddddddddddddddddddddddddddd"
)

type fakeRemote struct {
	byRevision map[string]source.SkillSnapshot
	calls      *int
}

func (f fakeRemote) ReadSkill(_ context.Context, _ source.Repository, _ string, revision string) (source.SkillSnapshot, error) {
	if f.calls != nil {
		(*f.calls)++
	}
	return f.byRevision[revision], nil
}

type failingRegistry struct {
	entry manifest.InstalledSkill
	err   error
}

func (f failingRegistry) ListProject(context.Context, string) ([]manifest.InstalledSkill, error) {
	return []manifest.InstalledSkill{f.entry}, nil
}

func (f failingRegistry) Advance(string, string, manifest.Skill, string, string) error { return f.err }

func TestPullCleanSkillAppliesExactRemoteAndAdvancesManifest(t *testing.T) {
	root, target, entry := managedProject(t, skillDocument("Old body\n"), map[string]string{"obsolete.txt": "old\n"})
	base := remoteSnapshot(baseCommit, baseTree, map[string][]byte{
		"SKILL.md": []byte(skillDocument("Old body\n")), "obsolete.txt": []byte("old\n"),
	}, nil)
	current := remoteSnapshot(remoteCommit, remoteTree, map[string][]byte{
		"SKILL.md":         []byte("---\nname:  sample\ndescription: Example skill.\n---\n\nNew body\n"),
		"scripts/check.sh": []byte("#!/bin/sh\nexit 0\n"),
	}, map[string]bool{"scripts/check.sh": true})
	service := newPullService(manifest.Store{}, fakeRemote{byRevision: map[string]source.SkillSnapshot{
		baseTree: base, "refs/heads/main": current,
	}})

	result, err := service.Pull(context.Background(), root, "sample")

	if err != nil {
		t.Fatalf("Pull() error = %v", err)
	}
	if !result.Changed || result.TreeSHA != remoteTree {
		t.Fatalf("result = %#v, want changed remote tree", result)
	}
	if _, err := os.Stat(filepath.Join(target, "obsolete.txt")); !os.IsNotExist(err) {
		t.Fatalf("obsolete file still exists or stat failed: %v", err)
	}
	content, err := os.ReadFile(filepath.Join(target, "SKILL.md"))
	if err != nil || string(content) != string(current.Files["SKILL.md"]) {
		t.Fatalf("SKILL.md = %q err=%v, want exact remote bytes", content, err)
	}
	info, err := os.Stat(filepath.Join(target, "scripts/check.sh"))
	if err != nil || info.Mode().Perm()&0o111 == 0 {
		t.Fatalf("script mode = %v err=%v, want executable", info.Mode(), err)
	}
	updated, err := (manifest.Store{}).Read(root)
	if err != nil {
		t.Fatal(err)
	}
	if updated.Skills[entry.Name].CommitSHA != remoteCommit || updated.Skills[entry.Name].TreeSHA != remoteTree {
		t.Fatalf("manifest entry = %#v, want remote baseline", updated.Skills[entry.Name])
	}
}

func TestPullAdvancesBaselineWhenTreeChangesWithEqualBytes(t *testing.T) {
	content := skillDocument("Same\n")
	root, target, entry := managedProject(t, content, nil)
	base := remoteSnapshot(baseCommit, baseTree, map[string][]byte{"SKILL.md": []byte(content)}, nil)
	current := remoteSnapshot(remoteCommit, remoteTree, map[string][]byte{"SKILL.md": []byte(content)}, nil)
	service := newPullService(manifest.Store{}, fakeRemote{byRevision: map[string]source.SkillSnapshot{
		baseTree: base, "refs/heads/main": current,
	}})

	result, err := service.Pull(context.Background(), root, entry.Name)

	if err != nil || !result.Changed {
		t.Fatalf("Pull() result=%#v error=%v, want manifest change", result, err)
	}
	got, _ := os.ReadFile(filepath.Join(target, "SKILL.md"))
	if string(got) != content {
		t.Fatalf("skill bytes changed: %q", got)
	}
	updated, _ := (manifest.Store{}).Read(root)
	if updated.Skills[entry.Name].TreeSHA != remoteTree {
		t.Fatalf("tree SHA = %q, want %q", updated.Skills[entry.Name].TreeSHA, remoteTree)
	}
}

func TestPullStopsBeforeRemoteReadWhenFileIsUntracked(t *testing.T) {
	root, target, _ := managedProject(t, skillDocument("Body\n"), nil)
	writeFile(t, filepath.Join(target, "notes.txt"), "untracked\n", 0o644)
	service := newPullService(manifest.Store{}, fakeRemote{byRevision: map[string]source.SkillSnapshot{}})

	_, err := service.Pull(context.Background(), root, "sample")

	if err == nil || !strings.Contains(err.Error(), "notes.txt") {
		t.Fatalf("Pull() error = %v, want untracked path", err)
	}
}

func TestPullRejectsTagBeforeRemoteRead(t *testing.T) {
	root, _, entry := managedProject(t, skillDocument("Body\n"), nil)
	entry.SourceRef = "refs/tags/v1.0.0"
	document, err := (manifest.Store{}).Read(root)
	if err != nil {
		t.Fatal(err)
	}
	document.Skills[entry.Name] = entry.Skill
	if err := (manifest.Store{}).Write(root, document); err != nil {
		t.Fatal(err)
	}
	remoteCalls := 0
	service := newPullService(manifest.Store{}, fakeRemote{byRevision: map[string]source.SkillSnapshot{}, calls: &remoteCalls})

	_, err = service.Pull(context.Background(), root, entry.Name)

	if err == nil || !strings.Contains(err.Error(), "fixed_source_ref") {
		t.Fatalf("Pull() error = %v", err)
	}
	if remoteCalls != 0 {
		t.Fatalf("remote calls = %d, want 0", remoteCalls)
	}
}

func TestPullRejectsRemoteNameDifferentFromManagedName(t *testing.T) {
	root, target, entry := managedProject(t, skillDocument("Old\n"), nil)
	base := remoteSnapshot(baseCommit, baseTree, map[string][]byte{"SKILL.md": []byte(skillDocument("Old\n"))}, nil)
	currentDocument := "---\nname: other\ndescription: Other skill.\n---\nRemote\n"
	current := remoteSnapshot(remoteCommit, remoteTree, map[string][]byte{"SKILL.md": []byte(currentDocument)}, nil)
	service := newPullService(manifest.Store{}, fakeRemote{byRevision: map[string]source.SkillSnapshot{
		baseTree: base, "refs/heads/main": current,
	}})

	_, err := service.Pull(context.Background(), root, entry.Name)

	if err == nil || !strings.Contains(err.Error(), "does not match managed name") {
		t.Fatalf("Pull() error = %v, want managed-name rejection", err)
	}
	content, readErr := os.ReadFile(filepath.Join(target, "SKILL.md"))
	if readErr != nil || string(content) != skillDocument("Old\n") {
		t.Fatalf("local skill changed: %q err=%v", content, readErr)
	}
}

func TestPullWritesMarkersAndAdvancesBaselineOnConflict(t *testing.T) {
	baseContent := skillDocument("Value: base\n")
	root, target, entry := managedProject(t, baseContent, map[string]string{"notes.txt": "Notes: base\n"})
	writeFile(t, filepath.Join(target, "SKILL.md"), skillDocument("Value: local\n"), 0o644)
	writeFile(t, filepath.Join(target, "notes.txt"), "Notes: local\n", 0o644)
	base := remoteSnapshot(baseCommit, baseTree, map[string][]byte{
		"SKILL.md":  []byte(baseContent),
		"notes.txt": []byte("Notes: base\n"),
	}, nil)
	current := remoteSnapshot(remoteCommit, remoteTree, map[string][]byte{
		"SKILL.md":  []byte(skillDocument("Value: remote\n")),
		"notes.txt": []byte("Notes: remote\n"),
	}, nil)
	service := newPullService(manifest.Store{}, fakeRemote{byRevision: map[string]source.SkillSnapshot{
		baseTree: base, "refs/heads/main": current,
	}})

	result, err := service.Pull(context.Background(), root, entry.Name)

	if !errors.Is(err, ErrConflict) || !result.Conflict || !result.Changed {
		t.Fatalf("Pull() result=%#v error=%v, want completed conflict", result, err)
	}
	wantConflictPaths := []string{
		".agents/skills/sample/SKILL.md",
		".agents/skills/sample/notes.txt",
	}
	if !slices.Equal(result.ConflictPaths, wantConflictPaths) {
		t.Fatalf("Pull() conflict paths=%v, want %v", result.ConflictPaths, wantConflictPaths)
	}
	content, readErr := os.ReadFile(filepath.Join(target, "SKILL.md"))
	if readErr != nil || !strings.Contains(string(content), "<<<<<<< gh-linked-skills:local") {
		t.Fatalf("conflict marker missing: %q err=%v", content, readErr)
	}
	updated, _ := (manifest.Store{}).Read(root)
	if updated.Skills[entry.Name].TreeSHA != remoteTree {
		t.Fatalf("tree SHA = %q, want conflict baseline %q", updated.Skills[entry.Name].TreeSHA, remoteTree)
	}
}

func TestPullRestoresSkillWhenManifestAdvanceFails(t *testing.T) {
	root, target, entry := managedProject(t, skillDocument("Old\n"), nil)
	base := remoteSnapshot(baseCommit, baseTree, map[string][]byte{"SKILL.md": []byte(skillDocument("Old\n"))}, nil)
	current := remoteSnapshot(remoteCommit, remoteTree, map[string][]byte{"SKILL.md": []byte(skillDocument("New\n"))}, nil)
	registry := failingRegistry{entry: entry, err: errors.New("manifest failed")}
	service := newPullService(registry, fakeRemote{byRevision: map[string]source.SkillSnapshot{
		baseTree: base, "refs/heads/main": current,
	}})

	_, err := service.Pull(context.Background(), root, entry.Name)

	if err == nil || !strings.Contains(err.Error(), "manifest failed") {
		t.Fatalf("Pull() error = %v, want manifest failure", err)
	}
	content, readErr := os.ReadFile(filepath.Join(target, "SKILL.md"))
	if readErr != nil || string(content) != skillDocument("Old\n") {
		t.Fatalf("original not restored: %q err=%v", content, readErr)
	}
}

func newPullService(registry Registry, remote Remote) *Service {
	return NewService(registry, workspace.Reader{}, remote, gitcli.New(command.New("git")), workspace.Writer{})
}

func managedProject(t *testing.T, document string, extra map[string]string) (string, string, manifest.InstalledSkill) {
	t.Helper()
	root := initProject(t)
	target := filepath.Join(root, ".agents", "skills", "sample")
	writeFile(t, filepath.Join(target, "SKILL.md"), document, 0o644)
	for path, content := range extra {
		writeFile(t, filepath.Join(target, filepath.FromSlash(path)), content, 0o644)
	}
	entry := manifest.Skill{
		Repository: "https://github.com/owner/repo.git", SourcePath: "skills/sample", SourceRef: "refs/heads/main",
		RefSHA: baseCommit, CommitSHA: baseCommit, TreeSHA: baseTree, Destination: ".agents/skills/sample",
	}
	if err := (manifest.Store{}).Write(root, manifest.Document{
		SchemaVersion: manifest.CurrentSchemaVersion, Skills: map[string]manifest.Skill{"sample": entry},
	}); err != nil {
		t.Fatal(err)
	}
	git(t, root, "add", ".agents/skills/sample", manifest.FileName)
	git(t, root, "commit", "-m", "track sample")
	return root, target, manifest.InstalledSkill{Name: "sample", Path: target, Skill: entry}
}

func initProject(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	git(t, root, "init", "-b", "main")
	git(t, root, "config", "user.name", "Test")
	git(t, root, "config", "user.email", "test@example.com")
	return root
}

func git(t *testing.T, root string, args ...string) {
	t.Helper()
	commandArgs := append([]string{"-C", root}, args...)
	command := exec.Command("git", commandArgs...)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, output)
	}
}

func writeFile(t *testing.T, path, content string, mode os.FileMode) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), mode); err != nil {
		t.Fatal(err)
	}
}

func skillDocument(body string) string {
	return "---\nname: sample\ndescription: Example skill.\n---\n\n" + body
}

func remoteSnapshot(commitSHA, treeSHA string, files map[string][]byte, executable map[string]bool) source.SkillSnapshot {
	if executable == nil {
		executable = make(map[string]bool)
	}
	return source.SkillSnapshot{CommitSHA: commitSHA, TreeSHA: treeSHA, Files: files, Executable: executable}
}
