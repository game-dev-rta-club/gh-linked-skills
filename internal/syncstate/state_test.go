package syncstate

import "testing"

func TestCalculate(t *testing.T) {
	base := Snapshot{"SKILL.md": []byte("base")}
	localChanged := Snapshot{"SKILL.md": []byte("local")}
	remoteChanged := Snapshot{"SKILL.md": []byte("remote")}

	tests := []struct {
		name     string
		local    Snapshot
		remote   Snapshot
		conflict bool
		want     State
	}{
		{name: "clean", local: base, remote: base, want: Clean},
		{name: "pull", local: base, remote: remoteChanged, want: Pull},
		{name: "push", local: localChanged, remote: base, want: Push},
		{name: "both changed", local: localChanged, remote: remoteChanged, want: Conflict},
		{name: "generated marker wins", local: base, remote: base, conflict: true, want: Conflict},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Calculate(base, tt.local, tt.remote, tt.conflict); got != tt.want {
				t.Fatalf("Calculate() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestSnapshotEqualityIncludesPaths(t *testing.T) {
	left := Snapshot{"scripts/check.mjs": []byte("same")}
	right := Snapshot{"scripts/other.mjs": []byte("same")}

	if left.Equal(right) {
		t.Fatal("Snapshot.Equal() = true for different paths")
	}
}

func TestCalculateChanges(t *testing.T) {
	if got := CalculateChanges(true, false, false); got != Push {
		t.Fatalf("CalculateChanges() = %q, want push", got)
	}
}

func TestHasGeneratedConflictMarker(t *testing.T) {
	tests := []struct {
		name    string
		content []byte
		want    bool
	}{
		{name: "generated", content: []byte("<<<<<<< gh-skill-linker:local\ntext\n"), want: true},
		{name: "opening removed", content: []byte("||||||| gh-skill-linker:base:abc\ntext\n>>>>>>> gh-skill-linker:remote:def\n"), want: true},
		{name: "ordinary git marker", content: []byte("<<<<<<< HEAD\ntext\n"), want: false},
		{name: "plain", content: []byte("text\n"), want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			snapshot := Snapshot{"SKILL.md": tt.content}
			if got := snapshot.HasGeneratedConflictMarker(); got != tt.want {
				t.Fatalf("HasGeneratedConflictMarker() = %v, want %v", got, tt.want)
			}
		})
	}
}
