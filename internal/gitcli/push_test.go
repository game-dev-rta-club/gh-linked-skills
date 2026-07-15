package gitcli

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/game-dev-rta-club/gh-linked-skills/internal/command"
	"github.com/game-dev-rta-club/gh-linked-skills/internal/source"
	"github.com/game-dev-rta-club/gh-linked-skills/internal/workspace"
)

func TestPublishSkillAddsMissingSubtreeAndPreservesRepository(t *testing.T) {
	bare := createBareRepository(t)
	client := New(command.New("git"))
	snapshot := publishSnapshot(t, "Published body\n")

	result, err := client.PublishSkill(
		context.Background(), bare, "main", "skills/sample", snapshot, "feat(skill): publish sample",
	)

	if err != nil {
		t.Fatal(err)
	}
	if !result.Pushed || result.TreeSHA != snapshot.TreeSHA || len(result.CommitSHA) != 40 {
		t.Fatalf("result = %#v", result)
	}
	if got := gitOutput(t, "--git-dir", bare, "show", "main:README.md"); got != "fixture\n" {
		t.Fatalf("README = %q", got)
	}
	if got := gitOutput(t, "--git-dir", bare, "show", "main:skills/sample/SKILL.md"); !strings.Contains(got, "Published body") {
		t.Fatalf("SKILL.md = %q", got)
	}
	mode := gitOutput(t, "--git-dir", bare, "ls-tree", "main", "skills/sample/scripts/check.sh")
	if !strings.HasPrefix(mode, "100755 ") {
		t.Fatalf("script mode = %q", mode)
	}
}

func TestPublishSkillAdoptsExactExistingSubtreeWithoutCommit(t *testing.T) {
	bare, _ := createBareSkillRepository(t)
	before := strings.TrimSpace(gitOutput(t, "--git-dir", bare, "rev-parse", "main"))
	client := New(command.New("git"))
	snapshot := source.SkillSnapshot{
		Files:      map[string][]byte{"SKILL.md": []byte("---\nname: sample\n---\n\nOld body\n")},
		Executable: map[string]bool{"SKILL.md": false},
	}
	snapshot.TreeSHA = treeSHA(t, snapshot)

	result, err := client.PublishSkill(
		context.Background(), bare, "main", "skills/sample", snapshot, "feat(skill): publish sample",
	)

	if err != nil {
		t.Fatal(err)
	}
	if result.Pushed || result.CommitSHA != before || result.TreeSHA != snapshot.TreeSHA {
		t.Fatalf("result = %#v", result)
	}
	after := strings.TrimSpace(gitOutput(t, "--git-dir", bare, "rev-parse", "main"))
	if after != before {
		t.Fatalf("commit changed: before=%s after=%s", before, after)
	}
}

func TestPublishSkillRefusesDifferentExistingSubtree(t *testing.T) {
	bare, _ := createBareSkillRepository(t)
	before := strings.TrimSpace(gitOutput(t, "--git-dir", bare, "rev-parse", "main"))
	client := New(command.New("git"))

	_, err := client.PublishSkill(
		context.Background(), bare, "main", "skills/sample", publishSnapshot(t, "Different\n"),
		"feat(skill): publish sample",
	)

	if !errors.Is(err, ErrTargetExists) {
		t.Fatalf("PublishSkill() error = %v", err)
	}
	after := strings.TrimSpace(gitOutput(t, "--git-dir", bare, "rev-parse", "main"))
	if after != before {
		t.Fatalf("commit changed: before=%s after=%s", before, after)
	}
}

func TestPublishSkillIncludesExecutableModeInExactComparison(t *testing.T) {
	bare := createBareRepository(t)
	client := New(command.New("git"))
	executable := publishSnapshot(t, "Same bytes\n")
	if _, err := client.PublishSkill(
		context.Background(), bare, "main", "skills/sample", executable, "feat(skill): publish sample",
	); err != nil {
		t.Fatal(err)
	}
	regular := publishSnapshot(t, "Same bytes\n")
	regular.Executable["scripts/check.sh"] = false
	regular.TreeSHA = treeSHA(t, regular)

	_, err := client.PublishSkill(
		context.Background(), bare, "main", "skills/sample", regular, "feat(skill): publish sample",
	)

	if !errors.Is(err, ErrTargetExists) {
		t.Fatalf("PublishSkill() error = %v", err)
	}
}

func TestPublishSkillRefusesFileAtTargetAncestor(t *testing.T) {
	bare := createBareRepositoryWithFiles(t, map[string]string{"skills": "not a directory\n"})
	client := New(command.New("git"))

	_, err := client.PublishSkill(
		context.Background(), bare, "main", "skills/sample", publishSnapshot(t, "Initial\n"),
		"feat(skill): publish sample",
	)

	if !errors.Is(err, ErrTargetExists) {
		t.Fatalf("PublishSkill() error = %v", err)
	}
}

func TestPublishSkillInitializesExplicitBranchInEmptyRepository(t *testing.T) {
	bare := filepath.Join(t.TempDir(), "empty.git")
	runGit(t, "init", "--bare", bare)
	runGit(t, "--git-dir", bare, "symbolic-ref", "HEAD", "refs/heads/main")
	client := New(command.New("git"))
	snapshot := publishSnapshot(t, "Initial\n")

	result, err := client.PublishSkill(
		context.Background(), bare, "main", "skills/sample", snapshot, "feat(skill): publish sample",
	)

	if err != nil {
		t.Fatal(err)
	}
	if !result.Pushed || result.TreeSHA != snapshot.TreeSHA {
		t.Fatalf("result = %#v", result)
	}
	if got := gitOutput(t, "--git-dir", bare, "show", "main:skills/sample/SKILL.md"); !strings.Contains(got, "Initial") {
		t.Fatalf("SKILL.md = %q", got)
	}
}

func TestPublishSkillRefusesMissingBranchInNonEmptyRepository(t *testing.T) {
	bare := createBareRepository(t)
	client := New(command.New("git"))

	_, err := client.PublishSkill(
		context.Background(), bare, "release", "skills/sample", publishSnapshot(t, "Initial\n"),
		"feat(skill): publish sample",
	)

	if !errors.Is(err, ErrBranchMissing) {
		t.Fatalf("PublishSkill() error = %v", err)
	}
	command := exec.Command("git", "--git-dir", bare, "show-ref", "--verify", "refs/heads/release")
	if err := command.Run(); err == nil {
		t.Fatal("release branch was created")
	}
}

func TestPublishSkillLosesSafelyToConcurrentBranchAdvance(t *testing.T) {
	bare := createBareRepository(t)
	competitor := filepath.Join(t.TempDir(), "competitor")
	runner := &beforePushRunner{delegate: command.New("git")}
	runner.before = func() {
		runGit(t, "clone", "--branch", "main", bare, competitor)
		runGit(t, "-C", competitor, "config", "user.name", "Concurrent")
		runGit(t, "-C", competitor, "config", "user.email", "concurrent@example.com")
		if err := os.WriteFile(filepath.Join(competitor, "other.txt"), []byte("concurrent\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		runGit(t, "-C", competitor, "add", "other.txt")
		runGit(t, "-C", competitor, "commit", "-m", "concurrent")
		runGit(t, "-C", competitor, "push", "origin", "main")
	}

	_, err := New(runner).PublishSkill(
		context.Background(), bare, "main", "skills/sample", publishSnapshot(t, "Initial\n"),
		"feat(skill): publish sample",
	)

	if !errors.Is(err, ErrRemoteChanged) {
		t.Fatalf("PublishSkill() error = %v", err)
	}
	if got := gitOutput(t, "--git-dir", bare, "show", "main:other.txt"); got != "concurrent\n" {
		t.Fatalf("concurrent file = %q", got)
	}
	command := exec.Command("git", "--git-dir", bare, "cat-file", "-e", "main:skills/sample")
	if err := command.Run(); err == nil {
		t.Fatal("stale publish created target subtree")
	}
}

func TestPushSkillCommitsSelectedSkillToBranch(t *testing.T) {
	bare, treeSHA := createBareSkillRepository(t)
	client := New(command.New("git"))
	files := source.SkillSnapshot{
		Files: map[string][]byte{
			"SKILL.md":         []byte("---\nname: sample\n---\n\nNew body\n"),
			"scripts/check.sh": []byte("#!/bin/sh\nexit 0\n"),
		},
		Executable: map[string]bool{"scripts/check.sh": true},
	}

	result, err := client.PushSkill(
		context.Background(), bare, "main", "skills/sample", treeSHA, files, "chore(skill): sync sample",
	)

	if err != nil {
		t.Fatalf("PushSkill() error = %v", err)
	}
	if !result.Pushed || result.TreeSHA == treeSHA || len(result.CommitSHA) != 40 {
		t.Fatalf("result = %#v, want pushed new tree", result)
	}
	body := gitOutput(t, "--git-dir", bare, "show", "main:skills/sample/SKILL.md")
	if !strings.Contains(body, "New body") {
		t.Fatalf("remote SKILL.md = %q, want new body", body)
	}
	mode := gitOutput(t, "--git-dir", bare, "ls-tree", "main", "skills/sample/scripts/check.sh")
	if !strings.HasPrefix(mode, "100755 ") {
		t.Fatalf("remote script mode = %q, want 100755", mode)
	}
}

func TestPushSkillStopsWhenClonedTreeChanged(t *testing.T) {
	bare, _ := createBareSkillRepository(t)
	client := New(command.New("git"))
	before := strings.TrimSpace(gitOutput(t, "--git-dir", bare, "rev-parse", "main"))

	_, err := client.PushSkill(
		context.Background(),
		bare,
		"main",
		"skills/sample",
		"stale-tree",
		source.SkillSnapshot{Files: map[string][]byte{"SKILL.md": []byte("---\nname: sample\n---\nNew\n")}},
		"chore(skill): sync sample",
	)

	if !errors.Is(err, ErrRemoteChanged) {
		t.Fatalf("PushSkill() error = %v, want ErrRemoteChanged", err)
	}
	after := strings.TrimSpace(gitOutput(t, "--git-dir", bare, "rev-parse", "main"))
	if after != before {
		t.Fatalf("remote branch mutated: before=%s after=%s", before, after)
	}
}

type beforePushRunner struct {
	delegate Runner
	once     sync.Once
	before   func()
}

type failCommandRunner struct {
	delegate Runner
	command  string
}

func (r failCommandRunner) Run(ctx context.Context, args ...string) (string, string, error) {
	for _, arg := range args {
		if arg == r.command {
			return "", "injected inspection failure", errors.New("injected failure")
		}
	}
	return r.delegate.Run(ctx, args...)
}

func TestPublishSkillStopsWhenTargetInspectionFails(t *testing.T) {
	bare := createBareRepository(t)
	before := strings.TrimSpace(gitOutput(t, "--git-dir", bare, "rev-parse", "main"))
	runner := failCommandRunner{delegate: command.New("git"), command: "cat-file"}

	_, err := New(runner).PublishSkill(
		context.Background(), bare, "main", "skills/sample", publishSnapshot(t, "Initial\n"),
		"feat(skill): publish sample",
	)

	if err == nil || !strings.Contains(err.Error(), "inspection") {
		t.Fatalf("PublishSkill() error = %v", err)
	}
	after := strings.TrimSpace(gitOutput(t, "--git-dir", bare, "rev-parse", "main"))
	if after != before {
		t.Fatalf("commit changed: before=%s after=%s", before, after)
	}
}

func (r *beforePushRunner) Run(ctx context.Context, args ...string) (string, string, error) {
	for _, arg := range args {
		if arg == "push" {
			r.once.Do(r.before)
			break
		}
	}
	return r.delegate.Run(ctx, args...)
}

func TestPushSkillDoesNotOverwriteConcurrentUnrelatedCommit(t *testing.T) {
	bare, treeSHA := createBareSkillRepository(t)
	competitor := filepath.Join(t.TempDir(), "competitor")
	runner := &beforePushRunner{delegate: command.New("git")}
	runner.before = func() {
		runGit(t, "clone", "--branch", "main", bare, competitor)
		runGit(t, "-C", competitor, "config", "user.name", "Concurrent")
		runGit(t, "-C", competitor, "config", "user.email", "concurrent@example.com")
		if err := os.WriteFile(filepath.Join(competitor, "unrelated.txt"), []byte("concurrent\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		runGit(t, "-C", competitor, "add", "unrelated.txt")
		runGit(t, "-C", competitor, "commit", "-m", "concurrent unrelated update")
		runGit(t, "-C", competitor, "push", "origin", "main")
	}
	client := New(runner)

	_, err := client.PushSkill(
		context.Background(), bare, "main", "skills/sample", treeSHA,
		source.SkillSnapshot{Files: map[string][]byte{
			"SKILL.md": []byte("---\nname: sample\ndescription: Sample skill.\n---\nNew\n"),
		}, Executable: map[string]bool{"SKILL.md": false}},
		"chore(skill): sync sample",
	)

	if !errors.Is(err, ErrRemoteChanged) {
		t.Fatalf("PushSkill() error = %v, want ErrRemoteChanged", err)
	}
	if got := gitOutput(t, "--git-dir", bare, "show", "main:unrelated.txt"); got != "concurrent\n" {
		t.Fatalf("concurrent commit missing: %q", got)
	}
	if got := gitOutput(t, "--git-dir", bare, "show", "main:skills/sample/SKILL.md"); strings.Contains(got, "New") {
		t.Fatalf("stale push overwrote remote skill: %q", got)
	}
}

func TestRemoteAdvancedClassifiesNonFastForwardOnly(t *testing.T) {
	if !remoteAdvanced("! [rejected] main -> main (fetch first)") {
		t.Fatal("remoteAdvanced() = false for fetch-first rejection")
	}
	if remoteAdvanced("remote: protected branch hook declined") {
		t.Fatal("remoteAdvanced() = true for branch protection")
	}
}

func createBareSkillRepository(t *testing.T) (string, string) {
	t.Helper()
	root := t.TempDir()
	work := filepath.Join(root, "work")
	if err := os.Mkdir(work, 0o755); err != nil {
		t.Fatal(err)
	}
	runGit(t, "-C", work, "init", "-b", "main")
	runGit(t, "-C", work, "config", "user.name", "Test")
	runGit(t, "-C", work, "config", "user.email", "test@example.com")
	path := filepath.Join(work, "skills", "sample", "SKILL.md")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("---\nname: sample\n---\n\nOld body\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit(t, "-C", work, "add", ".")
	runGit(t, "-C", work, "commit", "-m", "initial")
	treeSHA := strings.TrimSpace(gitOutput(t, "-C", work, "rev-parse", "HEAD:skills/sample"))
	bare := filepath.Join(root, "remote.git")
	runGit(t, "clone", "--bare", work, bare)
	return bare, treeSHA
}

func createBareRepository(t *testing.T) string {
	return createBareRepositoryWithFiles(t, map[string]string{"README.md": "fixture\n"})
}

func createBareRepositoryWithFiles(t *testing.T, files map[string]string) string {
	t.Helper()
	root := t.TempDir()
	work := filepath.Join(root, "work")
	if err := os.Mkdir(work, 0o755); err != nil {
		t.Fatal(err)
	}
	runGit(t, "-C", work, "init", "-b", "main")
	runGit(t, "-C", work, "config", "user.name", "Test")
	runGit(t, "-C", work, "config", "user.email", "test@example.com")
	for relative, content := range files {
		target := filepath.Join(work, filepath.FromSlash(relative))
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(target, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	runGit(t, "-C", work, "add", ".")
	runGit(t, "-C", work, "commit", "-m", "initial")
	bare := filepath.Join(root, "remote.git")
	runGit(t, "clone", "--bare", work, bare)
	return bare
}

func publishSnapshot(t *testing.T, body string) source.SkillSnapshot {
	t.Helper()
	snapshot := source.SkillSnapshot{
		Files: map[string][]byte{
			"SKILL.md":         []byte("---\nname: sample\ndescription: Sample skill.\n---\n" + body),
			"scripts/check.sh": []byte("#!/bin/sh\nexit 0\n"),
		},
		Executable: map[string]bool{"SKILL.md": false, "scripts/check.sh": true},
	}
	snapshot.TreeSHA = treeSHA(t, snapshot)
	return snapshot
}

func treeSHA(t *testing.T, snapshot source.SkillSnapshot) string {
	t.Helper()
	tree, err := workspace.TreeSHA(snapshot.Files, snapshot.Executable)
	if err != nil {
		t.Fatal(err)
	}
	return tree
}

func runGit(t *testing.T, args ...string) {
	t.Helper()
	command := exec.Command("git", args...)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, output)
	}
}

func gitOutput(t *testing.T, args ...string) string {
	t.Helper()
	command := exec.Command("git", args...)
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, output)
	}
	return string(output)
}
