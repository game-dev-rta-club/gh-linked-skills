package gitcli

import (
	"context"
	"strings"
	"testing"
)

func TestResolveRef(t *testing.T) {
	runner := &fakeRunner{stdout: "0123456789abcdef\trefs/heads/main\n"}

	sha, err := New(runner).ResolveRef(context.Background(), "https://github.com/owner/repo.git", "refs/heads/main")

	if err != nil {
		t.Fatalf("ResolveRef() error = %v", err)
	}
	if sha != "0123456789abcdef" {
		t.Fatalf("ResolveRef() = %q", sha)
	}
}

func TestFindRefReturnsMissingWithoutError(t *testing.T) {
	runner := &fakeRunner{}

	sha, found, err := New(runner).FindRef(
		context.Background(), "https://github.com/owner/repo.git", "refs/heads/proposal",
	)

	if err != nil || found || sha != "" {
		t.Fatalf("FindRef() = %q, %t, %v; want missing", sha, found, err)
	}
	if strings.Join(runner.args, " ") != "ls-remote https://github.com/owner/repo.git refs/heads/proposal" {
		t.Fatalf("runner args = %#v", runner.args)
	}
}

func TestFindRefReturnsExistingSHA(t *testing.T) {
	runner := &fakeRunner{stdout: "0123456789abcdef\trefs/heads/proposal\n"}

	sha, found, err := New(runner).FindRef(
		context.Background(), "https://github.com/owner/repo.git", "refs/heads/proposal",
	)

	if err != nil || !found || sha != "0123456789abcdef" {
		t.Fatalf("FindRef() = %q, %t, %v", sha, found, err)
	}
}
