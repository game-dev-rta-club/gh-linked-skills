package install

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/game-dev-rta-club/gh-linked-skills/internal/discovery"
	"github.com/game-dev-rta-club/gh-linked-skills/internal/manifest"
	"github.com/game-dev-rta-club/gh-linked-skills/internal/source"
	"github.com/game-dev-rta-club/gh-linked-skills/internal/workspace"
)

type failNamedWriter struct {
	name string
}

type failNthWriteStore struct {
	delegate manifest.Store
	failAt   int
	writes   int
}

func (s *failNthWriteStore) Read(projectRoot string) (manifest.Document, error) {
	return s.delegate.Read(projectRoot)
}

func (s *failNthWriteStore) Write(projectRoot string, document manifest.Document) error {
	s.writes++
	if s.writes == s.failAt {
		return fmt.Errorf("injected manifest write failure")
	}
	return s.delegate.Write(projectRoot, document)
}

func (w failNamedWriter) Install(target string, snapshot source.SkillSnapshot) error {
	if filepath.Base(target) == w.name {
		return fmt.Errorf("injected write failure")
	}
	return workspace.InstallSkill(target, snapshot)
}

func (w failNamedWriter) ReplaceExact(
	target string,
	snapshot source.SkillSnapshot,
	expected workspace.LocalSkill,
	commit func() error,
) error {
	if filepath.Base(target) == w.name {
		return fmt.Errorf("injected write failure")
	}
	return workspace.ReplaceExact(target, snapshot, expected, commit)
}

type fakeRemote struct {
	snapshot    source.SkillSnapshot
	snapshots   map[string]source.SkillSnapshot
	revisions   map[string]source.SkillSnapshot
	resolutions map[string]source.ResolvedRef
	discovery   discovery.Result
	revision    string
	resolvedRef string
	path        string
	calls       int
	discoveries int
}

func (f *fakeRemote) ResolveSourceRef(_ context.Context, _ source.Repository, ref string) (source.ResolvedRef, error) {
	f.resolvedRef = ref
	if resolved, ok := f.resolutions[ref]; ok {
		return resolved, nil
	}
	commitSHA := f.snapshot.CommitSHA
	if commitSHA == "" {
		commitSHA = f.discovery.CommitSHA
	}
	return source.ResolvedRef{RefSHA: commitSHA, CommitSHA: commitSHA}, nil
}

func (f *fakeRemote) ReadSkill(_ context.Context, _ source.Repository, skillPath, revision string) (source.SkillSnapshot, error) {
	f.calls++
	f.path = skillPath
	f.revision = revision
	if snapshot, ok := f.revisions[revision]; ok {
		return snapshot, nil
	}
	if snapshot, ok := f.snapshots[skillPath]; ok {
		return snapshot, nil
	}
	return f.snapshot, nil
}

func (f *fakeRemote) DiscoverSkills(_ context.Context, _ source.Repository, revision string) (discovery.Result, error) {
	f.discoveries++
	f.revision = revision
	return f.discovery, nil
}

func TestServiceInstallsExactSnapshotAndWritesManifest(t *testing.T) {
	root := t.TempDir()
	document := []byte("---\ndescription: Exact bytes stay here\nname: sample\n---\n\nBody\n")
	remote := &fakeRemote{snapshot: source.SkillSnapshot{
		CommitSHA: strings.Repeat("a", 40),
		TreeSHA:   strings.Repeat("b", 40),
		Files: map[string][]byte{
			"SKILL.md":         document,
			"scripts/check.sh": []byte("#!/bin/sh\necho ok\n"),
		},
		Executable: map[string]bool{"SKILL.md": false, "scripts/check.sh": true},
	}}
	service := NewService(remote, manifest.Store{}, workspace.Writer{})

	result, err := service.Install(context.Background(), root, "nikollson/sample-skills", "skills/sample", mainBranch(), Options{})
	if err != nil {
		t.Fatalf("Install() error = %v", err)
	}
	if !result.Installed || result.Name != "sample" || result.Path != ".agents/skills/sample" {
		t.Fatalf("result = %#v", result)
	}
	if remote.path != "skills/sample" || remote.revision != remote.snapshot.CommitSHA {
		t.Fatalf("remote request path=%q revision=%q", remote.path, remote.revision)
	}
	installedDocument, err := os.ReadFile(filepath.Join(root, ".agents/skills/sample/SKILL.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(installedDocument, document) {
		t.Fatalf("SKILL.md changed:\n%s", installedDocument)
	}
	info, err := os.Stat(filepath.Join(root, ".agents/skills/sample/scripts/check.sh"))
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o755 {
		t.Fatalf("script mode = %o, want 755", info.Mode().Perm())
	}

	managed, err := (manifest.Store{}).Read(root)
	if err != nil {
		t.Fatal(err)
	}
	entry := managed.Skills["sample"]
	if entry.Repository != "https://github.com/nikollson/sample-skills.git" || entry.SourcePath != "skills/sample" ||
		entry.SourceRef != "refs/heads/main" || entry.RefSHA != remote.snapshot.CommitSHA ||
		entry.CommitSHA != remote.snapshot.CommitSHA || entry.TreeSHA != remote.snapshot.TreeSHA ||
		entry.Destination != ".agents/skills/sample" {
		t.Fatalf("manifest entry = %#v", entry)
	}
}

func TestServiceSameInstallIsIdempotent(t *testing.T) {
	root := t.TempDir()
	remote := &fakeRemote{snapshot: sampleSnapshot()}
	service := NewService(remote, manifest.Store{}, workspace.Writer{})
	if _, err := service.Install(context.Background(), root, "nikollson/sample-skills", "skills/sample", mainBranch(), Options{}); err != nil {
		t.Fatal(err)
	}

	result, err := service.Install(context.Background(), root, "nikollson/sample-skills", "skills/sample", mainBranch(), Options{})
	if err != nil {
		t.Fatalf("second Install() error = %v", err)
	}
	if result.Installed {
		t.Fatalf("second Install() result = %#v, want no-op", result)
	}
}

func TestServiceInstallsAnnotatedTagAndRecordsRefIdentity(t *testing.T) {
	root := t.TempDir()
	commitSHA := strings.Repeat("c", 40)
	refSHA := strings.Repeat("d", 40)
	remote := &fakeRemote{
		snapshot: sampleSnapshotAt(commitSHA, strings.Repeat("e", 40), "sample"),
		resolutions: map[string]source.ResolvedRef{
			"refs/tags/v1.2.0": {RefSHA: refSHA, CommitSHA: commitSHA},
		},
	}
	service := NewService(remote, manifest.Store{}, workspace.Writer{})
	ref, err := source.NewRef(source.TagRef, "v1.2.0")
	if err != nil {
		t.Fatal(err)
	}

	result, err := service.Install(
		context.Background(), root, "nikollson/sample-skills", "skills/sample", ref, Options{},
	)

	if err != nil || !result.Installed {
		t.Fatalf("Install() = %#v, %v", result, err)
	}
	entry := readManagedSkill(t, root, "sample")
	if remote.resolvedRef != ref.FullName || remote.revision != commitSHA ||
		entry.SourceRef != ref.FullName || entry.RefSHA != refSHA || entry.CommitSHA != commitSHA {
		t.Fatalf("remote=%#v entry=%#v", remote, entry)
	}
}

func TestServiceReinstallRepinsCleanTagSkill(t *testing.T) {
	root := t.TempDir()
	remote := &fakeRemote{}
	service := NewService(remote, manifest.Store{}, workspace.Writer{})
	v1, _ := source.NewRef(source.TagRef, "v1.0.0")
	v2, _ := source.NewRef(source.TagRef, "v2.0.0")

	oldSnapshot := sampleSnapshotAt(strings.Repeat("a", 40), strings.Repeat("b", 40), "sample")
	remote.snapshot = oldSnapshot
	remote.revisions = map[string]source.SkillSnapshot{oldSnapshot.TreeSHA: oldSnapshot}
	remote.resolutions = map[string]source.ResolvedRef{
		v1.FullName: {RefSHA: strings.Repeat("a", 40), CommitSHA: strings.Repeat("a", 40)},
	}
	if _, err := service.Install(context.Background(), root, "nikollson/sample-skills", "skills/sample", v1, Options{}); err != nil {
		t.Fatal(err)
	}

	remote.snapshot = sampleSnapshotAt(strings.Repeat("c", 40), strings.Repeat("d", 40), "sample")
	remote.snapshot.Files["notes.md"] = []byte("version two\n")
	remote.snapshot.Executable["notes.md"] = false
	remote.resolutions = map[string]source.ResolvedRef{
		v2.FullName: {RefSHA: strings.Repeat("c", 40), CommitSHA: strings.Repeat("c", 40)},
	}
	result, err := service.Install(context.Background(), root, "nikollson/sample-skills", "skills/sample", v2, Options{})

	if err != nil || !result.Repinned || result.PreviousRef != v1.FullName || result.SourceRef != v2.FullName {
		t.Fatalf("repin result = %#v, %v", result, err)
	}
	if content, readErr := os.ReadFile(filepath.Join(root, ".agents/skills/sample/notes.md")); readErr != nil || string(content) != "version two\n" {
		t.Fatalf("repinned file = %q, %v", content, readErr)
	}
	entry := readManagedSkill(t, root, "sample")
	if entry.SourceRef != v2.FullName || entry.TreeSHA != strings.Repeat("d", 40) {
		t.Fatalf("repinned entry = %#v", entry)
	}
}

func TestServiceRepinRollsBackFilesAndManifestWhenManifestWriteFails(t *testing.T) {
	root := t.TempDir()
	remote := &fakeRemote{}
	store := &failNthWriteStore{failAt: 2}
	service := NewService(remote, store, workspace.Writer{})
	v1, _ := source.NewRef(source.TagRef, "v1.0.0")
	v2, _ := source.NewRef(source.TagRef, "v2.0.0")

	oldSnapshot := sampleSnapshotAt(strings.Repeat("a", 40), strings.Repeat("b", 40), "sample")
	oldSnapshot.Files["scripts/check.sh"] = []byte("#!/bin/sh\necho old\n")
	oldSnapshot.Executable["scripts/check.sh"] = true
	remote.snapshot = oldSnapshot
	remote.revisions = map[string]source.SkillSnapshot{oldSnapshot.TreeSHA: oldSnapshot}
	remote.resolutions = map[string]source.ResolvedRef{
		v1.FullName: {RefSHA: oldSnapshot.CommitSHA, CommitSHA: oldSnapshot.CommitSHA},
	}
	if _, err := service.Install(context.Background(), root, "nikollson/sample-skills", "skills/sample", v1, Options{}); err != nil {
		t.Fatal(err)
	}
	manifestBefore, err := os.ReadFile(filepath.Join(root, manifest.FileName))
	if err != nil {
		t.Fatal(err)
	}

	newSnapshot := sampleSnapshotAt(strings.Repeat("c", 40), strings.Repeat("d", 40), "sample")
	newSnapshot.Files["scripts/check.sh"] = []byte("#!/bin/sh\necho new\n")
	newSnapshot.Executable["scripts/check.sh"] = false
	remote.snapshot = newSnapshot
	remote.resolutions = map[string]source.ResolvedRef{
		v2.FullName: {RefSHA: newSnapshot.CommitSHA, CommitSHA: newSnapshot.CommitSHA},
	}

	_, err = service.Install(context.Background(), root, "nikollson/sample-skills", "skills/sample", v2, Options{})

	if err == nil || !strings.Contains(err.Error(), "injected manifest write failure") {
		t.Fatalf("Install() error = %v, want manifest write failure", err)
	}
	target := filepath.Join(root, ".agents/skills/sample")
	local, readErr := workspace.ReadSkill(target)
	if readErr != nil || !workspace.ExactSnapshot(local, oldSnapshot) {
		t.Fatalf("local skill was not rolled back: %#v, %v", local, readErr)
	}
	manifestAfter, readErr := os.ReadFile(filepath.Join(root, manifest.FileName))
	if readErr != nil || !bytes.Equal(manifestAfter, manifestBefore) {
		t.Fatalf("manifest changed after rollback: %q, %v", manifestAfter, readErr)
	}
}

func TestServiceMovedTagRequiresExplicitApproval(t *testing.T) {
	root := t.TempDir()
	remote := &fakeRemote{}
	service := NewService(remote, manifest.Store{}, workspace.Writer{})
	ref, _ := source.NewRef(source.TagRef, "v1.0.0")
	oldSnapshot := sampleSnapshotAt(strings.Repeat("a", 40), strings.Repeat("b", 40), "sample")
	remote.snapshot = oldSnapshot
	remote.revisions = map[string]source.SkillSnapshot{oldSnapshot.TreeSHA: oldSnapshot}
	remote.resolutions = map[string]source.ResolvedRef{
		ref.FullName: {RefSHA: strings.Repeat("a", 40), CommitSHA: oldSnapshot.CommitSHA},
	}
	if _, err := service.Install(context.Background(), root, "nikollson/sample-skills", "skills/sample", ref, Options{}); err != nil {
		t.Fatal(err)
	}

	newSnapshot := sampleSnapshotAt(strings.Repeat("c", 40), strings.Repeat("d", 40), "sample")
	newSnapshot.Files["notes.md"] = []byte("moved tag\n")
	newSnapshot.Executable["notes.md"] = false
	remote.snapshot = newSnapshot
	remote.resolutions[ref.FullName] = source.ResolvedRef{RefSHA: strings.Repeat("e", 40), CommitSHA: newSnapshot.CommitSHA}

	if _, err := service.Install(context.Background(), root, "nikollson/sample-skills", "skills/sample", ref, Options{}); err == nil || !strings.Contains(err.Error(), "tag_moved") {
		t.Fatalf("Install() error = %v, want tag_moved", err)
	}
	if _, err := os.Stat(filepath.Join(root, ".agents/skills/sample/notes.md")); !os.IsNotExist(err) {
		t.Fatalf("moved tag applied without approval: %v", err)
	}

	result, err := service.Install(
		context.Background(), root, "nikollson/sample-skills", "skills/sample", ref, Options{AcceptMovedTag: true},
	)
	if err != nil || !result.Repinned || result.PreviousRefSHA != strings.Repeat("a", 40) || result.RefSHA != strings.Repeat("e", 40) {
		t.Fatalf("approved moved tag = %#v, %v", result, err)
	}
}

func TestServiceMovedTagWithSameSnapshotStillRequiresExplicitApproval(t *testing.T) {
	root := t.TempDir()
	remote := &fakeRemote{}
	service := NewService(remote, manifest.Store{}, workspace.Writer{})
	ref, _ := source.NewRef(source.TagRef, "v1.0.0")
	snapshot := sampleSnapshotAt(strings.Repeat("a", 40), strings.Repeat("b", 40), "sample")
	oldRefSHA := strings.Repeat("c", 40)
	newRefSHA := strings.Repeat("d", 40)
	remote.snapshot = snapshot
	remote.revisions = map[string]source.SkillSnapshot{snapshot.TreeSHA: snapshot}
	remote.resolutions = map[string]source.ResolvedRef{
		ref.FullName: {RefSHA: oldRefSHA, CommitSHA: snapshot.CommitSHA},
	}
	if _, err := service.Install(context.Background(), root, "nikollson/sample-skills", "skills/sample", ref, Options{}); err != nil {
		t.Fatal(err)
	}

	remote.resolutions[ref.FullName] = source.ResolvedRef{RefSHA: newRefSHA, CommitSHA: snapshot.CommitSHA}
	if _, err := service.Install(context.Background(), root, "nikollson/sample-skills", "skills/sample", ref, Options{}); err == nil || !strings.Contains(err.Error(), "tag_moved") {
		t.Fatalf("Install() error = %v, want tag_moved", err)
	}
	if got := readManagedSkill(t, root, "sample").RefSHA; got != oldRefSHA {
		t.Fatalf("ref SHA after rejected moved tag = %s, want %s", got, oldRefSHA)
	}

	result, err := service.Install(
		context.Background(), root, "nikollson/sample-skills", "skills/sample", ref, Options{AcceptMovedTag: true},
	)
	if err != nil || !result.Repinned || result.PreviousRefSHA != oldRefSHA || result.RefSHA != newRefSHA {
		t.Fatalf("approved moved tag = %#v, %v", result, err)
	}
	entry := readManagedSkill(t, root, "sample")
	if entry.RefSHA != newRefSHA || entry.CommitSHA != snapshot.CommitSHA || entry.TreeSHA != snapshot.TreeSHA {
		t.Fatalf("manifest entry = %#v", entry)
	}
}

func TestServiceRepinRejectsLocalChangesWithoutMutation(t *testing.T) {
	root := t.TempDir()
	remote := &fakeRemote{}
	service := NewService(remote, manifest.Store{}, workspace.Writer{})
	v1, _ := source.NewRef(source.TagRef, "v1.0.0")
	v2, _ := source.NewRef(source.TagRef, "v2.0.0")
	oldSnapshot := sampleSnapshotAt(strings.Repeat("a", 40), strings.Repeat("b", 40), "sample")
	remote.snapshot = oldSnapshot
	remote.revisions = map[string]source.SkillSnapshot{oldSnapshot.TreeSHA: oldSnapshot}
	remote.resolutions = map[string]source.ResolvedRef{
		v1.FullName: {RefSHA: oldSnapshot.CommitSHA, CommitSHA: oldSnapshot.CommitSHA},
	}
	if _, err := service.Install(context.Background(), root, "nikollson/sample-skills", "skills/sample", v1, Options{}); err != nil {
		t.Fatal(err)
	}
	manifestBefore, err := os.ReadFile(filepath.Join(root, manifest.FileName))
	if err != nil {
		t.Fatal(err)
	}
	localPath := filepath.Join(root, ".agents/skills/sample/SKILL.md")
	localContent := []byte("---\nname: sample\ndescription: Local edit\n---\n")
	if err := os.WriteFile(localPath, localContent, 0o644); err != nil {
		t.Fatal(err)
	}
	newSnapshot := sampleSnapshotAt(strings.Repeat("c", 40), strings.Repeat("d", 40), "sample")
	remote.snapshot = newSnapshot
	remote.resolutions = map[string]source.ResolvedRef{
		v2.FullName: {RefSHA: newSnapshot.CommitSHA, CommitSHA: newSnapshot.CommitSHA},
	}

	_, err = service.Install(context.Background(), root, "nikollson/sample-skills", "skills/sample", v2, Options{})

	if err == nil || !strings.Contains(err.Error(), "local changes") {
		t.Fatalf("Install() error = %v, want local changes", err)
	}
	if content, readErr := os.ReadFile(localPath); readErr != nil || !bytes.Equal(content, localContent) {
		t.Fatalf("local file = %q, %v", content, readErr)
	}
	if content, readErr := os.ReadFile(filepath.Join(root, manifest.FileName)); readErr != nil || !bytes.Equal(content, manifestBefore) {
		t.Fatalf("manifest changed = %q, %v", content, readErr)
	}
}

func TestServiceRejectsMovedTagApprovalForDifferentTag(t *testing.T) {
	root := t.TempDir()
	remote := &fakeRemote{}
	service := NewService(remote, manifest.Store{}, workspace.Writer{})
	v1, _ := source.NewRef(source.TagRef, "v1.0.0")
	v2, _ := source.NewRef(source.TagRef, "v2.0.0")
	remote.snapshot = sampleSnapshot()
	remote.resolutions = map[string]source.ResolvedRef{
		v1.FullName: {RefSHA: remote.snapshot.CommitSHA, CommitSHA: remote.snapshot.CommitSHA},
	}
	if _, err := service.Install(context.Background(), root, "nikollson/sample-skills", "skills/sample", v1, Options{}); err != nil {
		t.Fatal(err)
	}
	remote.resolutions[v2.FullName] = source.ResolvedRef{RefSHA: remote.snapshot.CommitSHA, CommitSHA: remote.snapshot.CommitSHA}

	_, err := service.Install(
		context.Background(), root, "nikollson/sample-skills", "skills/sample", v2, Options{AcceptMovedTag: true},
	)

	if err == nil || !strings.Contains(err.Error(), "only valid when reinstalling the same tag") {
		t.Fatalf("Install() error = %v", err)
	}
}

func TestServiceAcceptsSkillDocumentPathAsDirectoryAlias(t *testing.T) {
	root := t.TempDir()
	remote := &fakeRemote{snapshot: sampleSnapshot()}
	service := NewService(remote, manifest.Store{}, workspace.Writer{})

	_, err := service.Install(
		context.Background(),
		root,
		"nikollson/sample-skills",
		"skills/sample/SKILL.md",
		mainBranch(),
		Options{},
	)

	if err != nil {
		t.Fatalf("Install() error = %v", err)
	}
	if remote.path != "skills/sample" {
		t.Fatalf("remote path = %q, want skills/sample", remote.path)
	}
	document, err := (manifest.Store{}).Read(root)
	if err != nil {
		t.Fatal(err)
	}
	if got := document.Skills["sample"].SourcePath; got != "skills/sample" {
		t.Fatalf("manifest source path = %q, want skills/sample", got)
	}
	result, err := service.Install(context.Background(), root, "nikollson/sample-skills", "skills/sample", mainBranch(), Options{})
	if err != nil || result.Installed {
		t.Fatalf("directory alias reinstall = %#v, %v, want no-op", result, err)
	}
}

func TestServiceInstallsDiscoveredSkillByNameFromPinnedCommit(t *testing.T) {
	root := t.TempDir()
	remote := &fakeRemote{
		discovery: discovery.Result{CommitSHA: strings.Repeat("c", 40), Skills: []discovery.Skill{
			{Name: "sample", Path: "skills/sample", TreeSHA: strings.Repeat("b", 40)},
		}},
		snapshots: map[string]source.SkillSnapshot{"skills/sample": sampleSnapshot()},
	}
	remote.snapshots["skills/sample"] = sampleSnapshotAt(strings.Repeat("c", 40), strings.Repeat("b", 40), "sample")
	service := NewService(remote, manifest.Store{}, workspace.Writer{})

	result, err := service.Install(context.Background(), root, "nikollson/sample-skills", "sample", mainBranch(), Options{})

	if err != nil || !result.Installed {
		t.Fatalf("Install() = %#v, %v", result, err)
	}
	if remote.discoveries != 1 || remote.calls != 1 || remote.path != "skills/sample" || remote.revision != strings.Repeat("c", 40) {
		t.Fatalf("remote = %#v", remote)
	}
}

func TestServiceDiscoverReturnsStableCandidates(t *testing.T) {
	remote := &fakeRemote{discovery: discovery.Result{CommitSHA: strings.Repeat("c", 40), Skills: []discovery.Skill{
		{Name: "alpha", Path: "skills/alpha"},
	}}}
	service := NewService(remote, manifest.Store{}, workspace.Writer{})

	result, err := service.Discover(context.Background(), "nikollson/sample-skills", mainBranch())

	if err != nil || len(result.Skills) != 1 || result.Skills[0].Name != "alpha" {
		t.Fatalf("Discover() = %#v, %v", result, err)
	}
	if remote.revision != strings.Repeat("c", 40) {
		t.Fatalf("revision = %q", remote.revision)
	}
}

func TestServiceInstallAllUsesOneDiscoveryCommit(t *testing.T) {
	root := t.TempDir()
	commit := strings.Repeat("c", 40)
	remote := &fakeRemote{
		discovery: discovery.Result{CommitSHA: commit, Skills: []discovery.Skill{
			{Name: "alpha", Path: "skills/alpha", TreeSHA: strings.Repeat("a", 40)},
			{Name: "beta", Path: "skills/beta", TreeSHA: strings.Repeat("b", 40)},
		}},
		snapshots: map[string]source.SkillSnapshot{
			"skills/alpha": sampleSnapshotAt(commit, strings.Repeat("a", 40), "alpha"),
			"skills/beta":  sampleSnapshotAt(commit, strings.Repeat("b", 40), "beta"),
		},
	}
	service := NewService(remote, manifest.Store{}, workspace.Writer{})

	results, err := service.InstallAll(context.Background(), root, "nikollson/sample-skills", mainBranch())

	if err != nil || len(results) != 2 || !results[0].Installed || !results[1].Installed {
		t.Fatalf("InstallAll() = %#v, %v", results, err)
	}
	if remote.discoveries != 1 || remote.calls != 2 || remote.revision != commit {
		t.Fatalf("remote = %#v", remote)
	}
	document, err := (manifest.Store{}).Read(root)
	if err != nil || len(document.Skills) != 2 {
		t.Fatalf("manifest = %#v, %v", document, err)
	}
}

func TestServiceInstallAllRejectsNameCollisionBeforeReadingSkills(t *testing.T) {
	root := t.TempDir()
	remote := &fakeRemote{discovery: discovery.Result{CommitSHA: strings.Repeat("c", 40), Skills: []discovery.Skill{
		{Name: "review", Namespace: "alice", Path: "skills/alice/review"},
		{Name: "review", Namespace: "bob", Path: "skills/bob/review"},
	}}}
	service := NewService(remote, manifest.Store{}, workspace.Writer{})

	_, err := service.InstallAll(context.Background(), root, "nikollson/sample-skills", mainBranch())

	if err == nil || !strings.Contains(err.Error(), "duplicate") {
		t.Fatalf("InstallAll() error = %v", err)
	}
	if remote.calls != 0 {
		t.Fatalf("skill reads = %d, want 0", remote.calls)
	}
	if _, statErr := os.Stat(filepath.Join(root, ".agents")); !os.IsNotExist(statErr) {
		t.Fatalf("workspace mutated: %v", statErr)
	}
}

func TestServiceInstallAllPreflightsEverySnapshotBeforeMutation(t *testing.T) {
	root := t.TempDir()
	commit := strings.Repeat("c", 40)
	remote := &fakeRemote{
		discovery: discovery.Result{CommitSHA: commit, Skills: []discovery.Skill{
			{Name: "alpha", Path: "skills/alpha", TreeSHA: strings.Repeat("a", 40)},
			{Name: "beta", Path: "skills/beta", TreeSHA: strings.Repeat("b", 40)},
		}},
		snapshots: map[string]source.SkillSnapshot{
			"skills/alpha": sampleSnapshotAt(commit, strings.Repeat("a", 40), "alpha"),
			"skills/beta":  sampleSnapshotAt(commit, strings.Repeat("b", 40), "wrong-name"),
		},
	}
	service := NewService(remote, manifest.Store{}, workspace.Writer{})

	_, err := service.InstallAll(context.Background(), root, "nikollson/sample-skills", mainBranch())

	if err == nil || !strings.Contains(err.Error(), "does not match") {
		t.Fatalf("InstallAll() error = %v", err)
	}
	if remote.calls != 2 {
		t.Fatalf("skill reads = %d, want 2", remote.calls)
	}
	if _, statErr := os.Stat(filepath.Join(root, ".agents")); !os.IsNotExist(statErr) {
		t.Fatalf("workspace mutated: %v", statErr)
	}
}

func TestServiceInstallAllReturnsSuccessfulSkillsWhenMutationLaterFails(t *testing.T) {
	root := t.TempDir()
	commit := strings.Repeat("c", 40)
	remote := &fakeRemote{
		discovery: discovery.Result{CommitSHA: commit, Skills: []discovery.Skill{
			{Name: "alpha", Path: "skills/alpha", TreeSHA: strings.Repeat("a", 40)},
			{Name: "beta", Path: "skills/beta", TreeSHA: strings.Repeat("b", 40)},
		}},
		snapshots: map[string]source.SkillSnapshot{
			"skills/alpha": sampleSnapshotAt(commit, strings.Repeat("a", 40), "alpha"),
			"skills/beta":  sampleSnapshotAt(commit, strings.Repeat("b", 40), "beta"),
		},
	}
	service := NewService(remote, manifest.Store{}, failNamedWriter{name: "beta"})

	results, err := service.InstallAll(context.Background(), root, "nikollson/sample-skills", mainBranch())

	if err == nil || !strings.Contains(err.Error(), "injected write failure") {
		t.Fatalf("InstallAll() error = %v", err)
	}
	if len(results) != 1 || results[0].Name != "alpha" || !results[0].Installed {
		t.Fatalf("InstallAll() results = %#v", results)
	}
	document, readErr := (manifest.Store{}).Read(root)
	if readErr != nil || len(document.Skills) != 1 {
		t.Fatalf("manifest = %#v, %v", document, readErr)
	}
	if _, statErr := os.Stat(filepath.Join(root, ".agents/skills/beta")); !os.IsNotExist(statErr) {
		t.Fatalf("failed skill exists: %v", statErr)
	}
}

func TestServiceRejectsSkillNameThatDiffersFromSourceDirectory(t *testing.T) {
	root := t.TempDir()
	service := NewService(&fakeRemote{snapshot: sampleSnapshot()}, manifest.Store{}, workspace.Writer{})

	_, err := service.Install(
		context.Background(),
		root,
		"nikollson/sample-skills",
		"skills/different-name",
		mainBranch(),
		Options{},
	)

	if err == nil || !strings.Contains(err.Error(), "source directory") {
		t.Fatalf("Install() error = %v, want source directory mismatch", err)
	}
	if _, statErr := os.Stat(filepath.Join(root, ".agents/skills/sample")); !os.IsNotExist(statErr) {
		t.Fatalf("skill exists after rejected install: %v", statErr)
	}
}

func TestServiceRejectsRepositoryRootSkillDocument(t *testing.T) {
	root := t.TempDir()
	remote := &fakeRemote{snapshot: sampleSnapshot()}
	service := NewService(remote, manifest.Store{}, workspace.Writer{})

	_, err := service.Install(
		context.Background(),
		root,
		"nikollson/sample-skills",
		"SKILL.md",
		mainBranch(),
		Options{},
	)

	if err == nil || !strings.Contains(err.Error(), "repository-root SKILL.md is not supported") {
		t.Fatalf("Install() error = %v, want repository-root rejection", err)
	}
	if remote.calls != 0 {
		t.Fatalf("remote calls = %d, want 0", remote.calls)
	}
	if _, statErr := os.Stat(filepath.Join(root, manifest.FileName)); !os.IsNotExist(statErr) {
		t.Fatalf("manifest exists after rejected install: %v", statErr)
	}
}

func TestServiceRejectsUnmanagedExistingDestination(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, ".agents/skills/sample")
	if err := os.MkdirAll(target, 0o755); err != nil {
		t.Fatal(err)
	}
	original := []byte("keep me\n")
	if err := os.WriteFile(filepath.Join(target, "SKILL.md"), original, 0o644); err != nil {
		t.Fatal(err)
	}
	service := NewService(&fakeRemote{snapshot: sampleSnapshot()}, manifest.Store{}, workspace.Writer{})

	if _, err := service.Install(context.Background(), root, "nikollson/sample-skills", "skills/sample", mainBranch(), Options{}); err == nil {
		t.Fatal("Install() error = nil, want destination conflict")
	}
	got, err := os.ReadFile(filepath.Join(target, "SKILL.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, original) {
		t.Fatalf("existing destination changed to %q", got)
	}
	if _, err := os.Stat(filepath.Join(root, manifest.FileName)); !os.IsNotExist(err) {
		t.Fatalf("manifest exists after rejected install: %v", err)
	}
}

func TestServiceRejectsSymlinkedAgentDirectoryWithoutOutsideWrite(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(root, ".agents")); err != nil {
		t.Fatal(err)
	}
	service := NewService(&fakeRemote{snapshot: sampleSnapshot()}, manifest.Store{}, workspace.Writer{})

	_, err := service.Install(context.Background(), root, "nikollson/sample-skills", "skills/sample", mainBranch(), Options{})

	if err == nil || !strings.Contains(err.Error(), "symlink") {
		t.Fatalf("Install() error = %v, want symlink rejection", err)
	}
	if _, statErr := os.Stat(filepath.Join(outside, "skills", "sample")); !os.IsNotExist(statErr) {
		t.Fatalf("install escaped project: %v", statErr)
	}
}

func TestServiceRejectsLFSPointerWithoutMutation(t *testing.T) {
	root := t.TempDir()
	snapshot := sampleSnapshot()
	snapshot.Files["assets/model.bin"] = []byte("version https://git-lfs.github.com/spec/v1\noid sha256:" + strings.Repeat("c", 64) + "\nsize 42\n")
	snapshot.Executable["assets/model.bin"] = false
	service := NewService(&fakeRemote{snapshot: snapshot}, manifest.Store{}, workspace.Writer{})

	if _, err := service.Install(context.Background(), root, "nikollson/sample-skills", "skills/sample", mainBranch(), Options{}); err == nil {
		t.Fatal("Install() error = nil, want LFS rejection")
	}
	if _, err := os.Stat(filepath.Join(root, ".agents/skills/sample")); !os.IsNotExist(err) {
		t.Fatalf("skill exists after LFS rejection: %v", err)
	}
}

func TestServiceRejectsHTTPSRepositoryURL(t *testing.T) {
	service := NewService(&fakeRemote{snapshot: sampleSnapshot()}, manifest.Store{}, workspace.Writer{})

	_, err := service.Install(
		context.Background(),
		t.TempDir(),
		"https://github.com/nikollson/sample-skills",
		"skills/sample",
		mainBranch(),
		Options{},
	)

	if err == nil || !strings.Contains(err.Error(), "OWNER/REPO") {
		t.Fatalf("Install() error = %v, want OWNER/REPO rejection", err)
	}
}

func sampleSnapshot() source.SkillSnapshot {
	return sampleSnapshotAt(strings.Repeat("a", 40), strings.Repeat("b", 40), "sample")
}

func sampleSnapshotAt(commitSHA, treeSHA, name string) source.SkillSnapshot {
	return source.SkillSnapshot{
		CommitSHA: commitSHA,
		TreeSHA:   treeSHA,
		Files: map[string][]byte{
			"SKILL.md": []byte("---\nname: " + name + "\ndescription: Sample skill\n---\n\nBody\n"),
		},
		Executable: map[string]bool{"SKILL.md": false},
	}
}

func readManagedSkill(t *testing.T, root, name string) manifest.Skill {
	t.Helper()
	document, err := (manifest.Store{}).Read(root)
	if err != nil {
		t.Fatal(err)
	}
	return document.Skills[name]
}

func mainBranch() source.Ref {
	ref, err := source.NewRef(source.BranchRef, "main")
	if err != nil {
		panic(err)
	}
	return ref
}
