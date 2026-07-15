package gitcli

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
)

type fakeRunner struct {
	stdout string
	stderr string
	err    error
	args   []string
}

func (f *fakeRunner) Run(_ context.Context, args ...string) (string, string, error) {
	f.args = append([]string(nil), args...)
	return f.stdout, f.stderr, f.err
}

func TestRoot(t *testing.T) {
	runner := &fakeRunner{stdout: "/repo\n"}

	root, err := New(runner).Root(context.Background())

	if err != nil {
		t.Fatalf("Root() error = %v", err)
	}
	if root != "/repo" {
		t.Fatalf("Root() = %q, want /repo", root)
	}
	if strings.Join(runner.args, " ") != "rev-parse --show-toplevel" {
		t.Fatalf("runner args = %#v", runner.args)
	}
}

func TestRootReportsNotRepository(t *testing.T) {
	runner := &fakeRunner{stderr: "fatal: not a git repository", err: errors.New("exit status 128")}

	_, err := New(runner).Root(context.Background())

	if err == nil || !strings.Contains(err.Error(), "not a git repository") {
		t.Fatalf("Root() error = %v, want git context", err)
	}
}

func TestTrackedFiles(t *testing.T) {
	runner := &fakeRunner{stdout: ".agents/skills/sample/SKILL.md\x00.agents/skills/sample/scripts/check.mjs\x00"}

	files, err := New(runner).TrackedFiles(context.Background(), "/repo", ".agents/skills/sample")

	if err != nil {
		t.Fatalf("TrackedFiles() error = %v", err)
	}
	want := []string{
		".agents/skills/sample/SKILL.md",
		".agents/skills/sample/scripts/check.mjs",
	}
	if !reflect.DeepEqual(files, want) {
		t.Fatalf("TrackedFiles() = %#v, want %#v", files, want)
	}
	wantArgs := []string{"-C", "/repo", "ls-files", "-z", "--cached", "--", ".agents/skills/sample"}
	if !reflect.DeepEqual(runner.args, wantArgs) {
		t.Fatalf("runner args = %#v, want %#v", runner.args, wantArgs)
	}
}

func TestPushFilesIncludesTrackedAndUntrackedNonIgnored(t *testing.T) {
	runner := &fakeRunner{stdout: ".agents/skills/sample/SKILL.md\x00.agents/skills/sample/new.mjs\x00"}

	files, err := New(runner).PushFiles(context.Background(), "/repo", ".agents/skills/sample")

	if err != nil {
		t.Fatalf("PushFiles() error = %v", err)
	}
	if len(files) != 2 {
		t.Fatalf("PushFiles() = %#v, want 2 files", files)
	}
	wantArgs := []string{"-C", "/repo", "ls-files", "-z", "--cached", "--others", "--exclude-standard", "--", ".agents/skills/sample"}
	if !reflect.DeepEqual(runner.args, wantArgs) {
		t.Fatalf("runner args = %#v, want %#v", runner.args, wantArgs)
	}
}
