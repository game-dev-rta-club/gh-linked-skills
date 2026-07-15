package push

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/game-dev-rta-club/gh-linked-skills/internal/gitcli"
	"github.com/game-dev-rta-club/gh-linked-skills/internal/manifest"
	"github.com/game-dev-rta-club/gh-linked-skills/internal/source"
	"github.com/game-dev-rta-club/gh-linked-skills/internal/syncstate"
	"github.com/game-dev-rta-club/gh-linked-skills/internal/workspace"
)

const (
	baseCommit = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	baseTree   = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	newCommit  = "cccccccccccccccccccccccccccccccccccccccc"
	newTree    = "dddddddddddddddddddddddddddddddddddddddd"
)

type fakeRegistry struct {
	entry      manifest.InstalledSkill
	advanceErr error
	advanced   bool
	commitSHA  string
	treeSHA    string
}

func (f *fakeRegistry) ListProject(context.Context, string) ([]manifest.InstalledSkill, error) {
	return []manifest.InstalledSkill{f.entry}, nil
}

func (f *fakeRegistry) Advance(_ string, _ string, _ manifest.Skill, commitSHA, treeSHA string) error {
	if f.advanceErr != nil {
		return f.advanceErr
	}
	f.advanced = true
	f.commitSHA = commitSHA
	f.treeSHA = treeSHA
	return nil
}

type fakeLocalReader struct{ local workspace.LocalSkill }

func (f fakeLocalReader) Read(string) (workspace.LocalSkill, error) { return f.local, nil }

type fakeRemote struct {
	current source.SkillSnapshot
	canPush bool
	err     error
}

func (f fakeRemote) ReadSkill(context.Context, source.Repository, string, string) (source.SkillSnapshot, error) {
	return f.current, f.err
}

func (f fakeRemote) ReadPermission(context.Context, source.Repository, string) (bool, error) {
	return f.canPush, f.err
}

type fakeInventory struct{ files []string }

func (f fakeInventory) PushFiles(context.Context, string, string) ([]string, error) {
	return f.files, nil
}

type fakePusher struct {
	result   gitcli.PushResult
	err      error
	calls    int
	snapshot source.SkillSnapshot
}

func (f *fakePusher) PushSkill(_ context.Context, _, _, _, _ string, snapshot source.SkillSnapshot, _ string) (gitcli.PushResult, error) {
	f.calls++
	f.snapshot = snapshot
	return f.result, f.err
}

func TestPushSendsExactLocalBytesAndAdvancesManifest(t *testing.T) {
	registry := newRegistry()
	local := localSkill("---\nname:  sample\n---\nChanged\n", false, nil)
	remote := remoteSkill("---\nname: sample\n---\nOld\n", false, baseCommit, baseTree)
	pusher := &fakePusher{result: gitcli.PushResult{CommitSHA: newCommit, TreeSHA: newTree, Pushed: true}}
	service := newPushService(registry, local, fakeRemote{current: remote, canPush: true}, pusher,
		[]string{".agents/skills/sample/SKILL.md"})

	result, err := service.Push(context.Background(), "/repo", "sample")

	if err != nil {
		t.Fatalf("Push() error = %v", err)
	}
	if !result.Pushed || result.TreeSHA != newTree || pusher.calls != 1 {
		t.Fatalf("result=%#v calls=%d, want one push", result, pusher.calls)
	}
	if !bytes.Equal(pusher.snapshot.Files["SKILL.md"], local.Files["SKILL.md"]) {
		t.Fatalf("pushed bytes = %q, want exact local %q", pusher.snapshot.Files["SKILL.md"], local.Files["SKILL.md"])
	}
	if !registry.advanced || registry.commitSHA != newCommit || registry.treeSHA != newTree {
		t.Fatalf("registry = %#v, want pushed baseline", registry)
	}
}

func TestPushDoesNotIgnoreExecutableModeOnlyChange(t *testing.T) {
	registry := newRegistry()
	local := localSkill("---\nname: sample\n---\nBody\n", true, nil)
	remote := remoteSkill(string(local.Files["SKILL.md"]), false, baseCommit, baseTree)
	pusher := &fakePusher{result: gitcli.PushResult{CommitSHA: newCommit, TreeSHA: newTree, Pushed: true}}
	service := newPushService(registry, local, fakeRemote{current: remote, canPush: true}, pusher,
		[]string{".agents/skills/sample/SKILL.md"})

	_, err := service.Push(context.Background(), "/repo", "sample")

	if err != nil || pusher.calls != 1 || !pusher.snapshot.Executable["SKILL.md"] {
		t.Fatalf("Push() error=%v calls=%d modes=%v, want executable push", err, pusher.calls, pusher.snapshot.Executable)
	}
}

func TestPushExactRemoteIsNoOpAndRefreshesCommitBaseline(t *testing.T) {
	registry := newRegistry()
	local := localSkill("---\nname: sample\n---\nBody\n", false, nil)
	remote := remoteSkill(string(local.Files["SKILL.md"]), false, newCommit, baseTree)
	pusher := &fakePusher{}
	service := newPushService(registry, local, fakeRemote{current: remote, canPush: true}, pusher,
		[]string{".agents/skills/sample/SKILL.md"})

	result, err := service.Push(context.Background(), "/repo", "sample")

	if err != nil || result.Pushed || pusher.calls != 0 {
		t.Fatalf("Push() result=%#v error=%v calls=%d, want no-op", result, err, pusher.calls)
	}
	if !registry.advanced || registry.commitSHA != newCommit || registry.treeSHA != baseTree {
		t.Fatalf("registry = %#v, want refreshed commit baseline", registry)
	}
}

func TestPushReadOnlyStopsBeforePusher(t *testing.T) {
	registry := newRegistry()
	local := localSkill("---\nname: sample\n---\nChanged\n", false, nil)
	pusher := &fakePusher{}
	service := newPushService(registry, local, fakeRemote{canPush: false}, pusher,
		[]string{".agents/skills/sample/SKILL.md"})

	_, err := service.Push(context.Background(), "/repo", "sample")

	if err == nil || !strings.Contains(err.Error(), "repository_read_only") || pusher.calls != 0 {
		t.Fatalf("Push() error=%v calls=%d, want read-only rejection", err, pusher.calls)
	}
}

func TestPushRejectsTagBeforeInspectingLocalChanges(t *testing.T) {
	registry := newRegistry()
	registry.entry.SourceRef = "refs/tags/v1.0.0"
	local := localSkill("---\nname: other\n---\nChanged\n", false, nil)
	pusher := &fakePusher{}
	service := newPushService(registry, local, fakeRemote{canPush: true}, pusher,
		[]string{".agents/skills/sample/SKILL.md"})

	_, err := service.Push(context.Background(), "/repo", "sample")

	if err == nil || !strings.Contains(err.Error(), "source_ref_read_only") || pusher.calls != 0 {
		t.Fatalf("Push() error=%v calls=%d, want tag read-only rejection", err, pusher.calls)
	}
}

func TestPushRejectsLocalNameDifferentFromManagedName(t *testing.T) {
	registry := newRegistry()
	local := localSkill("---\nname: other\n---\nChanged\n", false, nil)
	pusher := &fakePusher{}
	service := newPushService(registry, local, fakeRemote{canPush: true}, pusher,
		[]string{".agents/skills/sample/SKILL.md"})

	_, err := service.Push(context.Background(), "/repo", "sample")

	if err == nil || !strings.Contains(err.Error(), "invalid_local_skill") || pusher.calls != 0 {
		t.Fatalf("Push() error=%v calls=%d, want managed-name rejection", err, pusher.calls)
	}
}

func TestPushStaleRemoteStopsBeforePusher(t *testing.T) {
	registry := newRegistry()
	local := localSkill("---\nname: sample\n---\nChanged\n", false, nil)
	remote := remoteSkill("---\nname: sample\n---\nRemote\n", false, newCommit, newTree)
	pusher := &fakePusher{}
	service := newPushService(registry, local, fakeRemote{current: remote, canPush: true}, pusher,
		[]string{".agents/skills/sample/SKILL.md"})

	_, err := service.Push(context.Background(), "/repo", "sample")

	if !errors.Is(err, ErrRemoteChanged) || pusher.calls != 0 {
		t.Fatalf("Push() error=%v calls=%d, want stale rejection", err, pusher.calls)
	}
}

func TestPushIgnoredFileStopsBeforePusher(t *testing.T) {
	registry := newRegistry()
	local := localSkill("---\nname: sample\n---\nChanged\n", false, map[string][]byte{"private.txt": []byte("ignored\n")})
	pusher := &fakePusher{}
	service := newPushService(registry, local, fakeRemote{canPush: true}, pusher,
		[]string{".agents/skills/sample/SKILL.md"})

	_, err := service.Push(context.Background(), "/repo", "sample")

	if err == nil || !strings.Contains(err.Error(), "private.txt") || pusher.calls != 0 {
		t.Fatalf("Push() error=%v calls=%d, want ignored-file rejection", err, pusher.calls)
	}
}

func TestPushManifestFailureWarnsNotToRetry(t *testing.T) {
	registry := newRegistry()
	registry.advanceErr = errors.New("disk full")
	local := localSkill("---\nname: sample\n---\nChanged\n", false, nil)
	remote := remoteSkill("---\nname: sample\n---\nOld\n", false, baseCommit, baseTree)
	pusher := &fakePusher{result: gitcli.PushResult{CommitSHA: newCommit, TreeSHA: newTree, Pushed: true}}
	service := newPushService(registry, local, fakeRemote{current: remote, canPush: true}, pusher,
		[]string{".agents/skills/sample/SKILL.md"})

	_, err := service.Push(context.Background(), "/repo", "sample")

	if !errors.Is(err, ErrMetadataUpdate) || pusher.calls != 1 || !strings.Contains(err.Error(), "do not retry") {
		t.Fatalf("Push() error=%v calls=%d, want reconciliation warning", err, pusher.calls)
	}
}

func newRegistry() *fakeRegistry {
	entry := manifest.Skill{
		Repository: "https://github.com/owner/repo.git", SourcePath: "skills/sample", SourceRef: "refs/heads/main",
		RefSHA: baseCommit, CommitSHA: baseCommit, TreeSHA: baseTree, Destination: ".agents/skills/sample",
	}
	return &fakeRegistry{entry: manifest.InstalledSkill{Name: "sample", Path: "/repo/.agents/skills/sample", Skill: entry}}
}

func newPushService(registry Registry, local workspace.LocalSkill, remote Remote, pusher Pusher, files []string) *Service {
	return NewService(registry, fakeLocalReader{local: local}, remote, fakeInventory{files: files}, pusher)
}

func localSkill(document string, executable bool, additional map[string][]byte) workspace.LocalSkill {
	document = withDescription(document)
	files := map[string][]byte{"SKILL.md": []byte(document)}
	modes := map[string]bool{"SKILL.md": executable}
	for path, content := range additional {
		files[path] = content
		modes[path] = false
	}
	return workspace.LocalSkill{Files: files, Executable: modes, Snapshot: syncstate.Snapshot(files)}
}

func remoteSkill(document string, executable bool, commitSHA, treeSHA string) source.SkillSnapshot {
	document = withDescription(document)
	return source.SkillSnapshot{
		CommitSHA: commitSHA, TreeSHA: treeSHA,
		Files:      map[string][]byte{"SKILL.md": []byte(document)},
		Executable: map[string]bool{"SKILL.md": executable},
	}
}

func withDescription(document string) string {
	if strings.Contains(document, "\ndescription:") {
		return document
	}
	return strings.Replace(document, "\n---\n", "\ndescription: Example skill.\n---\n", 1)
}
