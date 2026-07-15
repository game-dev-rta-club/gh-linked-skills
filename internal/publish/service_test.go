package publish

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/game-dev-rta-club/gh-skill-linker/internal/gitcli"
	"github.com/game-dev-rta-club/gh-skill-linker/internal/manifest"
	"github.com/game-dev-rta-club/gh-skill-linker/internal/source"
	"github.com/game-dev-rta-club/gh-skill-linker/internal/workspace"
)

type fakeRegistry struct {
	document manifest.Document
	addErrs  []error
	added    []manifest.Skill
}

func (f *fakeRegistry) Read(string) (manifest.Document, error) {
	return f.document, nil
}

func (f *fakeRegistry) Add(_ string, name string, _ manifest.Document, entry manifest.Skill) error {
	f.added = append(f.added, entry)
	if len(f.addErrs) > 0 {
		err := f.addErrs[0]
		f.addErrs = f.addErrs[1:]
		if err != nil {
			return err
		}
	}
	if f.document.Skills == nil {
		f.document.Skills = make(map[string]manifest.Skill)
	}
	f.document.Skills[name] = entry
	return nil
}

type fakePermission struct {
	canPush bool
	err     error
	calls   int
}

func (f *fakePermission) ReadPermission(context.Context, source.Repository, string) (bool, error) {
	f.calls++
	return f.canPush, f.err
}

type fakeInventory struct {
	files []string
	err   error
}

func (f fakeInventory) PushFiles(context.Context, string, string) ([]string, error) {
	return f.files, f.err
}

type fakePublisher struct {
	results  []gitcli.PushResult
	errs     []error
	calls    int
	url      string
	branch   string
	path     string
	snapshot source.SkillSnapshot
}

func (f *fakePublisher) PublishSkill(
	_ context.Context,
	repositoryURL, branch, skillPath string,
	snapshot source.SkillSnapshot,
	_ string,
) (gitcli.PushResult, error) {
	index := f.calls
	f.calls++
	f.url, f.branch, f.path, f.snapshot = repositoryURL, branch, skillPath, snapshot
	var result gitcli.PushResult
	if index < len(f.results) {
		result = f.results[index]
	}
	if index < len(f.errs) {
		return result, f.errs[index]
	}
	return result, nil
}

func TestPublishAddsUnmanagedSkillAndRecordsRemoteBaseline(t *testing.T) {
	root, localPath := createPublishSkill(t, "sample")
	registry := &fakeRegistry{document: emptyManifest()}
	permission := &fakePermission{canPush: true}
	publisher := &fakePublisher{results: []gitcli.PushResult{{
		CommitSHA: strings.Repeat("a", 40), TreeSHA: strings.Repeat("b", 40), Pushed: true,
	}}}
	service := NewService(registry, workspace.Reader{}, permission, fakeInventory{files: []string{
		".agents/skills/sample/SKILL.md", ".agents/skills/sample/scripts/check.sh",
	}}, publisher)

	result, err := service.Publish(
		context.Background(), root, "nikollson/skills", "sample", branchRef("main"),
	)

	if err != nil {
		t.Fatal(err)
	}
	if !result.Published || result.Path != ".agents/skills/sample" || result.SourcePath != "skills/sample" {
		t.Fatalf("result = %#v", result)
	}
	if publisher.calls != 1 || publisher.url != "https://github.com/nikollson/skills.git" ||
		publisher.branch != "main" || publisher.path != "skills/sample" {
		t.Fatalf("publisher = %#v", publisher)
	}
	if string(publisher.snapshot.Files["SKILL.md"]) == "" || !publisher.snapshot.Executable["scripts/check.sh"] {
		t.Fatalf("snapshot = %#v", publisher.snapshot)
	}
	if publisher.snapshot.TreeSHA == "" {
		t.Fatal("snapshot tree SHA is empty")
	}
	entry := registry.document.Skills["sample"]
	if entry.Repository != publisher.url || entry.SourcePath != "skills/sample" ||
		entry.SourceRef != "refs/heads/main" || entry.RefSHA != strings.Repeat("a", 40) ||
		entry.CommitSHA != strings.Repeat("a", 40) || entry.TreeSHA != strings.Repeat("b", 40) ||
		entry.Destination != ".agents/skills/sample" {
		t.Fatalf("manifest entry = %#v", entry)
	}
	if localPath != filepath.Join(root, filepath.FromSlash(result.Path)) {
		t.Fatalf("local path = %q, result path = %q", localPath, result.Path)
	}
}

func TestPublishRejectsManagedSkillBeforeRemoteAccess(t *testing.T) {
	root, _ := createPublishSkill(t, "sample")
	registry := &fakeRegistry{document: emptyManifest()}
	registry.document.Skills["sample"] = validManifestSkill("old", "sample")
	permission := &fakePermission{canPush: true}
	publisher := &fakePublisher{}
	service := NewService(registry, workspace.Reader{}, permission, fakeInventory{}, publisher)

	_, err := service.Publish(context.Background(), root, "nikollson/new", "sample", branchRef("main"))

	if !errors.Is(err, ErrAlreadyManaged) || !strings.Contains(err.Error(), "use push") {
		t.Fatalf("Publish() error = %v", err)
	}
	if permission.calls != 0 || publisher.calls != 0 {
		t.Fatalf("remote calls: permission=%d publisher=%d", permission.calls, publisher.calls)
	}
}

func TestPublishRejectsInvalidLocalSkillBeforeRemoteAccess(t *testing.T) {
	root, localPath := createPublishSkill(t, "sample")
	document := "---\nname: different\ndescription: Example skill.\n---\nBody\n"
	if err := os.WriteFile(filepath.Join(localPath, "SKILL.md"), []byte(document), 0o644); err != nil {
		t.Fatal(err)
	}
	permission := &fakePermission{canPush: true}
	publisher := &fakePublisher{}
	service := NewService(
		&fakeRegistry{document: emptyManifest()}, workspace.Reader{}, permission, publishInventory(), publisher,
	)

	_, err := service.Publish(context.Background(), root, "nikollson/skills", "sample", branchRef("main"))

	if err == nil || !strings.Contains(err.Error(), "invalid_local_skill") {
		t.Fatalf("Publish() error = %v", err)
	}
	if permission.calls != 0 || publisher.calls != 0 {
		t.Fatalf("remote calls: permission=%d publisher=%d", permission.calls, publisher.calls)
	}
}

func TestPublishRejectsIgnoredLocalFiles(t *testing.T) {
	root, _ := createPublishSkill(t, "sample")
	service := NewService(
		&fakeRegistry{document: emptyManifest()}, workspace.Reader{}, &fakePermission{canPush: true},
		fakeInventory{files: []string{".agents/skills/sample/SKILL.md"}}, &fakePublisher{},
	)

	_, err := service.Publish(context.Background(), root, "nikollson/skills", "sample", branchRef("main"))

	if err == nil || !strings.Contains(err.Error(), "ignored_files") || !strings.Contains(err.Error(), "scripts/check.sh") {
		t.Fatalf("Publish() error = %v", err)
	}
}

func TestPublishRequiresExactOwnerRepositoryInput(t *testing.T) {
	root, _ := createPublishSkill(t, "sample")
	for _, repository := range []string{" owner/repo", "owner/repo.git", "owner/repo/extra", "./repo"} {
		service := NewService(
			&fakeRegistry{document: emptyManifest()}, workspace.Reader{}, &fakePermission{canPush: true},
			publishInventory(), &fakePublisher{},
		)
		if _, err := service.Publish(context.Background(), root, repository, "sample", branchRef("main")); err == nil {
			t.Errorf("Publish(%q) error = nil", repository)
		}
	}
}

func TestPublishRejectsReadOnlyRepositoryBeforeGitMutation(t *testing.T) {
	root, _ := createPublishSkill(t, "sample")
	registry := &fakeRegistry{document: emptyManifest()}
	publisher := &fakePublisher{}
	service := NewService(
		registry, workspace.Reader{}, &fakePermission{canPush: false}, publishInventory(), publisher,
	)

	_, err := service.Publish(context.Background(), root, "nikollson/skills", "sample", branchRef("main"))

	if err == nil || !strings.Contains(err.Error(), "repository_read_only") || publisher.calls != 0 || len(registry.added) != 0 {
		t.Fatalf("error=%v publisher calls=%d manifest=%#v", err, publisher.calls, registry.document)
	}
}

func TestPublishLeavesManifestUnchangedWhenRemoteFails(t *testing.T) {
	root, _ := createPublishSkill(t, "sample")
	registry := &fakeRegistry{document: emptyManifest()}
	publisher := &fakePublisher{errs: []error{errors.New("push failed")}}
	service := NewService(registry, workspace.Reader{}, &fakePermission{canPush: true}, publishInventory(), publisher)

	_, err := service.Publish(context.Background(), root, "nikollson/skills", "sample", branchRef("main"))

	if err == nil || len(registry.added) != 0 || len(registry.document.Skills) != 0 {
		t.Fatalf("error=%v manifest=%#v", err, registry.document)
	}
}

func TestPublishRetriesByAdoptingExactRemoteAfterManifestFailure(t *testing.T) {
	root, _ := createPublishSkill(t, "sample")
	registry := &fakeRegistry{document: emptyManifest(), addErrs: []error{errors.New("manifest busy"), nil}}
	publisher := &fakePublisher{results: []gitcli.PushResult{
		{CommitSHA: strings.Repeat("a", 40), TreeSHA: strings.Repeat("b", 40), Pushed: true},
		{CommitSHA: strings.Repeat("a", 40), TreeSHA: strings.Repeat("b", 40), Pushed: false},
	}}
	service := NewService(registry, workspace.Reader{}, &fakePermission{canPush: true}, publishInventory(), publisher)

	first, firstErr := service.Publish(context.Background(), root, "nikollson/skills", "sample", branchRef("main"))
	second, secondErr := service.Publish(context.Background(), root, "nikollson/skills", "sample", branchRef("main"))

	if !errors.Is(firstErr, ErrMetadataUpdate) || !first.Published {
		t.Fatalf("first result=%#v error=%v", first, firstErr)
	}
	if secondErr != nil || second.Published || publisher.calls != 2 {
		t.Fatalf("second result=%#v error=%v calls=%d", second, secondErr, publisher.calls)
	}
	if _, ok := registry.document.Skills["sample"]; !ok {
		t.Fatalf("manifest = %#v", registry.document)
	}
}

func createPublishSkill(t *testing.T, name string) (string, string) {
	t.Helper()
	root := t.TempDir()
	path := filepath.Join(root, ".agents", "skills", name)
	if err := os.MkdirAll(filepath.Join(path, "scripts"), 0o755); err != nil {
		t.Fatal(err)
	}
	document := "---\nname: " + name + "\ndescription: Example skill.\n---\nBody\n"
	if err := os.WriteFile(filepath.Join(path, "SKILL.md"), []byte(document), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(path, "scripts", "check.sh"), []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	return root, path
}

func emptyManifest() manifest.Document {
	return manifest.Document{SchemaVersion: manifest.CurrentSchemaVersion, Skills: map[string]manifest.Skill{}}
}

func validManifestSkill(repository, name string) manifest.Skill {
	return manifest.Skill{
		Repository: "https://github.com/nikollson/" + repository + ".git",
		SourcePath: "skills/" + name, SourceRef: "refs/heads/main",
		RefSHA: strings.Repeat("a", 40), CommitSHA: strings.Repeat("a", 40), TreeSHA: strings.Repeat("b", 40),
		Destination: ".agents/skills/" + name,
	}
}

func branchRef(name string) source.Ref {
	ref, err := source.NewRef(source.BranchRef, name)
	if err != nil {
		panic(err)
	}
	return ref
}

func publishInventory() fakeInventory {
	return fakeInventory{files: []string{
		".agents/skills/sample/SKILL.md", ".agents/skills/sample/scripts/check.sh",
	}}
}
