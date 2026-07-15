package gitcli

import (
	"context"
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
