package gitcli

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/game-dev-rta-club/gh-linked-skills/internal/command"
)

func TestMergeFileCombinesNonOverlappingChanges(t *testing.T) {
	client := New(command.New("git"))
	base := []byte("first\nsecond\nthird\n")
	local := []byte("local first\nsecond\nthird\n")
	remote := []byte("first\nsecond\nremote third\n")

	merged, conflict, err := client.MergeFile(context.Background(), local, base, remote, "base-sha", "remote-sha")

	if err != nil {
		t.Fatalf("MergeFile() error = %v", err)
	}
	if conflict {
		t.Fatal("MergeFile() conflict = true, want false")
	}
	for _, want := range [][]byte{[]byte("local first"), []byte("remote third")} {
		if !bytes.Contains(merged, want) {
			t.Fatalf("merged = %q, want %q", merged, want)
		}
	}
}

func TestMergeFileLeavesDiff3Markers(t *testing.T) {
	client := New(command.New("git"))
	base := []byte("value: base\n")
	local := []byte("value: local\n")
	remote := []byte("value: remote\n")

	merged, conflict, err := client.MergeFile(context.Background(), local, base, remote, "base-sha", "remote-sha")

	if err != nil {
		t.Fatalf("MergeFile() error = %v", err)
	}
	if !conflict {
		t.Fatal("MergeFile() conflict = false, want true")
	}
	for _, want := range []string{
		"<<<<<<< gh-linked-skills:local",
		"||||||| gh-linked-skills:base:base-sha",
		"=======",
		">>>>>>> gh-linked-skills:remote:remote-sha",
	} {
		if !strings.Contains(string(merged), want) {
			t.Fatalf("merged = %q, want marker %q", merged, want)
		}
	}
}

func TestMergeFileHandlesMultipleConflicts(t *testing.T) {
	client := New(command.New("git"))
	base := []byte("start\nfirst: base\nmiddle\nsecond: base\nend\n")
	local := []byte("start\nfirst: local\nmiddle\nsecond: local\nend\n")
	remote := []byte("start\nfirst: remote\nmiddle\nsecond: remote\nend\n")

	merged, conflict, err := client.MergeFile(context.Background(), local, base, remote, "base-sha", "remote-sha")

	if err != nil {
		t.Fatalf("MergeFile() error = %v", err)
	}
	if !conflict {
		t.Fatal("MergeFile() conflict = false, want true")
	}
	if got := strings.Count(string(merged), "<<<<<<< gh-linked-skills:local"); got != 2 {
		t.Fatalf("conflict marker count = %d, want 2\nmerged = %q", got, merged)
	}
}
