package uninstall

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/game-dev-rta-club/gh-skill-linker/internal/manifest"
	"github.com/game-dev-rta-club/gh-skill-linker/internal/workspace"
)

func TestUninstallRemovesCleanSkillAndManifestEntry(t *testing.T) {
	root := t.TempDir()
	entry := writeManagedSkill(t, root, "sample", "original\n")
	other := entry
	other.SourcePath = "skills/other"
	other.Destination = ".agents/skills/other"
	store := manifest.Store{}
	if err := store.Write(root, manifest.Document{SchemaVersion: manifest.CurrentSchemaVersion, Skills: map[string]manifest.Skill{
		"sample": entry,
		"other":  other,
	}}); err != nil {
		t.Fatal(err)
	}

	result, err := NewService(store, workspace.Reader{}, workspace.Writer{}).Uninstall(context.Background(), root, "sample", Options{})

	if err != nil {
		t.Fatalf("Uninstall() error = %v", err)
	}
	if result.Name != "sample" || result.Path != ".agents/skills/sample" {
		t.Fatalf("result = %#v", result)
	}
	if _, err := os.Lstat(filepath.Join(root, ".agents", "skills", "sample")); !os.IsNotExist(err) {
		t.Fatalf("skill still exists: %v", err)
	}
	document, err := store.Read(root)
	if err != nil {
		t.Fatal(err)
	}
	if _, exists := document.Skills["sample"]; exists || len(document.Skills) != 1 {
		t.Fatalf("document = %#v, want only other", document)
	}
}

func TestUninstallRejectsLocalChangesWithoutForce(t *testing.T) {
	root := t.TempDir()
	entry := writeManagedSkill(t, root, "sample", "original\n")
	store := writeManifest(t, root, "sample", entry)
	path := filepath.Join(root, ".agents", "skills", "sample", "SKILL.md")
	if err := os.WriteFile(path, []byte("changed\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := NewService(store, workspace.Reader{}, workspace.Writer{}).Uninstall(context.Background(), root, "sample", Options{})

	if err == nil || !strings.Contains(err.Error(), "local changes") || !strings.Contains(err.Error(), "--force") {
		t.Fatalf("Uninstall() error = %v, want local changes and --force guidance", err)
	}
	if _, statErr := os.Stat(path); statErr != nil {
		t.Fatalf("changed skill was removed: %v", statErr)
	}
	document, readErr := store.Read(root)
	if readErr != nil || len(document.Skills) != 1 {
		t.Fatalf("manifest changed: document=%#v error=%v", document, readErr)
	}
}

func TestUninstallRejectsAddedEmptyDirectoryWithoutForce(t *testing.T) {
	root := t.TempDir()
	entry := writeManagedSkill(t, root, "sample", "original\n")
	store := writeManifest(t, root, "sample", entry)
	empty := filepath.Join(root, ".agents", "skills", "sample", "notes")
	if err := os.Mkdir(empty, 0o755); err != nil {
		t.Fatal(err)
	}

	_, err := NewService(store, workspace.Reader{}, workspace.Writer{}).Uninstall(context.Background(), root, "sample", Options{})

	if err == nil || !strings.Contains(err.Error(), "local changes") {
		t.Fatalf("Uninstall() error = %v, want local changes", err)
	}
	if _, statErr := os.Stat(empty); statErr != nil {
		t.Fatalf("empty directory was removed: %v", statErr)
	}
}

func TestUninstallForceRemovesLocalChanges(t *testing.T) {
	root := t.TempDir()
	entry := writeManagedSkill(t, root, "sample", "original\n")
	store := writeManifest(t, root, "sample", entry)
	path := filepath.Join(root, ".agents", "skills", "sample", "SKILL.md")
	if err := os.WriteFile(path, []byte("changed\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := NewService(store, workspace.Reader{}, workspace.Writer{}).Uninstall(context.Background(), root, "sample", Options{Force: true})

	if err != nil {
		t.Fatalf("Uninstall() error = %v", err)
	}
	if _, err := os.Lstat(filepath.Dir(path)); !os.IsNotExist(err) {
		t.Fatalf("skill still exists: %v", err)
	}
}

func TestUninstallMissingDestinationRemovesStaleManifestEntry(t *testing.T) {
	root := t.TempDir()
	entry := validEntry("sample", strings.Repeat("a", 40))
	store := writeManifest(t, root, "sample", entry)

	result, err := NewService(store, workspace.Reader{}, workspace.Writer{}).Uninstall(context.Background(), root, ".agents/skills/sample", Options{})

	if err != nil || !result.DestinationMissing {
		t.Fatalf("result=%#v error=%v", result, err)
	}
	document, readErr := store.Read(root)
	if readErr != nil || len(document.Skills) != 0 {
		t.Fatalf("document=%#v error=%v", document, readErr)
	}
}

func TestUninstallRejectsUnmanagedSkill(t *testing.T) {
	root := t.TempDir()

	_, err := NewService(manifest.Store{}, workspace.Reader{}, workspace.Writer{}).Uninstall(context.Background(), root, "sample", Options{})

	if err == nil || !strings.Contains(err.Error(), "not found") {
		t.Fatalf("Uninstall() error = %v, want not found", err)
	}
}

func TestUninstallForceRejectsNonDirectoryDestination(t *testing.T) {
	root := t.TempDir()
	entry := validEntry("sample", strings.Repeat("a", 40))
	store := writeManifest(t, root, "sample", entry)
	target := filepath.Join(root, ".agents", "skills", "sample")
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(target, []byte("not a directory\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := NewService(store, workspace.Reader{}, workspace.Writer{}).Uninstall(context.Background(), root, "sample", Options{Force: true})

	if err == nil || !strings.Contains(err.Error(), "not a directory") {
		t.Fatalf("Uninstall() error = %v, want non-directory rejection", err)
	}
	if _, statErr := os.Stat(target); statErr != nil {
		t.Fatalf("destination was removed: %v", statErr)
	}
}

func TestUninstallRollsBackDestinationWhenManifestCommitFails(t *testing.T) {
	root := t.TempDir()
	entry := writeManagedSkill(t, root, "sample", "original\n")
	registry := failingRegistry{entry: manifest.InstalledSkill{
		Name: "sample", Path: filepath.Join(root, ".agents", "skills", "sample"), Skill: entry,
	}, err: errors.New("manifest failed")}

	_, err := NewService(registry, workspace.Reader{}, workspace.Writer{}).Uninstall(context.Background(), root, "sample", Options{})

	if err == nil || !strings.Contains(err.Error(), "manifest failed") {
		t.Fatalf("Uninstall() error = %v", err)
	}
	if _, statErr := os.Stat(filepath.Join(root, ".agents", "skills", "sample", "SKILL.md")); statErr != nil {
		t.Fatalf("skill was not restored: %v", statErr)
	}
}

type failingRegistry struct {
	entry manifest.InstalledSkill
	err   error
}

func (f failingRegistry) ListProject(context.Context, string) ([]manifest.InstalledSkill, error) {
	return []manifest.InstalledSkill{f.entry}, nil
}

func (f failingRegistry) Remove(string, string, manifest.Skill) error { return f.err }

func writeManagedSkill(t *testing.T, root, name, content string) manifest.Skill {
	t.Helper()
	directory := filepath.Join(root, ".agents", "skills", name)
	if err := os.MkdirAll(directory, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(directory, "SKILL.md"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	local, err := workspace.ReadSkill(directory)
	if err != nil {
		t.Fatal(err)
	}
	treeSHA, err := workspace.TreeSHA(local.Files, local.Executable)
	if err != nil {
		t.Fatal(err)
	}
	return validEntry(name, treeSHA)
}

func validEntry(name, treeSHA string) manifest.Skill {
	return manifest.Skill{
		Repository: "https://github.com/owner/repository.git", SourcePath: "skills/" + name,
		SourceRef: "refs/heads/main", RefSHA: strings.Repeat("a", 40), CommitSHA: strings.Repeat("a", 40),
		TreeSHA: treeSHA, Destination: ".agents/skills/" + name,
	}
}

func writeManifest(t *testing.T, root, name string, entry manifest.Skill) manifest.Store {
	t.Helper()
	store := manifest.Store{}
	if err := store.Write(root, manifest.Document{SchemaVersion: manifest.CurrentSchemaVersion, Skills: map[string]manifest.Skill{name: entry}}); err != nil {
		t.Fatal(err)
	}
	return store
}
