package merge

import (
	"bytes"
	"context"
	"errors"
	"testing"

	"github.com/game-dev-rta-club/gh-linked-skills/internal/command"
	"github.com/game-dev-rta-club/gh-linked-skills/internal/gitcli"
	"github.com/game-dev-rta-club/gh-linked-skills/internal/source"
	"github.com/game-dev-rta-club/gh-linked-skills/internal/workspace"
)

func TestThreeWayMergesNonOverlappingTextChanges(t *testing.T) {
	base := snapshot(t, "base-tree", map[string][]byte{
		"SKILL.md":  []byte("---\nname: sample\n---\n\nBody\n"),
		"notes.txt": []byte("first\nsecond\nthird\n"),
	})
	local := localSkill(t, map[string][]byte{
		"SKILL.md":  []byte(installed("Body\n")),
		"notes.txt": []byte("local first\nsecond\nthird\n"),
	})
	remote := snapshot(t, "remote-tree", map[string][]byte{
		"SKILL.md":  []byte("---\nname: sample\n---\n\nBody\n"),
		"notes.txt": []byte("first\nsecond\nremote third\n"),
	})

	merged, conflictPaths, err := ThreeWay(context.Background(), gitcli.New(command.New("git")), base, local, remote)

	if err != nil {
		t.Fatalf("ThreeWay() error = %v", err)
	}
	if len(conflictPaths) != 0 {
		t.Fatalf("ThreeWay() conflict paths = %v, want none", conflictPaths)
	}
	for _, want := range [][]byte{[]byte("local first"), []byte("remote third")} {
		if !bytes.Contains(merged.Files["notes.txt"], want) {
			t.Fatalf("merged notes = %q, want %q", merged.Files["notes.txt"], want)
		}
	}
}

func TestThreeWayLeavesMarkersForOverlappingTextChanges(t *testing.T) {
	base := snapshot(t, "base-tree", map[string][]byte{
		"SKILL.md":  []byte("---\nname: sample\n---\n\nBody\n"),
		"notes.txt": []byte("value: base\n"),
	})
	local := localSkill(t, map[string][]byte{
		"SKILL.md":  []byte(installed("Body\n")),
		"notes.txt": []byte("value: local\n"),
	})
	remote := snapshot(t, "remote-tree", map[string][]byte{
		"SKILL.md":  []byte("---\nname: sample\n---\n\nBody\n"),
		"notes.txt": []byte("value: remote\n"),
	})

	merged, conflictPaths, err := ThreeWay(context.Background(), gitcli.New(command.New("git")), base, local, remote)

	if err != nil {
		t.Fatalf("ThreeWay() error = %v", err)
	}
	if len(conflictPaths) != 1 || conflictPaths[0] != "notes.txt" || !bytes.Contains(merged.Files["notes.txt"], []byte("<<<<<<< gh-linked-skills:local")) {
		t.Fatalf("conflict paths = %v, notes = %q", conflictPaths, merged.Files["notes.txt"])
	}
}

func TestThreeWayLeavesMarkersForSkillDocumentConflict(t *testing.T) {
	base := snapshot(t, "base-tree", map[string][]byte{"SKILL.md": []byte("---\nname: sample\n---\n\nBase body\n")})
	local := localSkill(t, map[string][]byte{"SKILL.md": []byte(installed("Local body\n"))})
	remote := snapshot(t, "remote-tree", map[string][]byte{"SKILL.md": []byte("---\nname: sample\n---\n\nRemote body\n")})

	merged, conflictPaths, err := ThreeWay(context.Background(), gitcli.New(command.New("git")), base, local, remote)

	if err != nil {
		t.Fatalf("ThreeWay() error = %v", err)
	}
	if len(conflictPaths) != 1 || conflictPaths[0] != "SKILL.md" {
		t.Fatalf("ThreeWay() conflict paths = %v, want SKILL.md", conflictPaths)
	}
	if !bytes.Contains(merged.Files["SKILL.md"], []byte("<<<<<<< gh-linked-skills:local")) {
		t.Fatalf("merged document has no marker:\n%s", merged.Files["SKILL.md"])
	}
}

func TestThreeWayRejectsModifyDeleteConflict(t *testing.T) {
	base := snapshot(t, "base-tree", map[string][]byte{
		"SKILL.md":  []byte("---\nname: sample\n---\n\nBody\n"),
		"notes.txt": []byte("base\n"),
	})
	local := localSkill(t, map[string][]byte{
		"SKILL.md":  []byte(installed("Body\n")),
		"notes.txt": []byte("local\n"),
	})
	remote := snapshot(t, "remote-tree", map[string][]byte{"SKILL.md": []byte("---\nname: sample\n---\n\nBody\n")})

	_, _, err := ThreeWay(context.Background(), gitcli.New(command.New("git")), base, local, remote)

	if !errors.Is(err, ErrUnsupportedConflict) {
		t.Fatalf("ThreeWay() error = %v, want ErrUnsupportedConflict", err)
	}
}

func TestThreeWayRejectsBinaryConflict(t *testing.T) {
	base := snapshot(t, "base-tree", map[string][]byte{
		"SKILL.md":  []byte("---\nname: sample\n---\n\nBody\n"),
		"asset.bin": []byte{'a', 0, 'b'},
	})
	local := localSkill(t, map[string][]byte{
		"SKILL.md":  []byte(installed("Body\n")),
		"asset.bin": []byte{'l', 0, 'b'},
	})
	remote := snapshot(t, "remote-tree", map[string][]byte{
		"SKILL.md":  []byte("---\nname: sample\n---\n\nBody\n"),
		"asset.bin": []byte{'r', 0, 'b'},
	})

	_, _, err := ThreeWay(context.Background(), gitcli.New(command.New("git")), base, local, remote)

	if !errors.Is(err, ErrUnsupportedConflict) {
		t.Fatalf("ThreeWay() error = %v, want ErrUnsupportedConflict", err)
	}
}

func TestThreeWayCombinesContentAndExecutableChanges(t *testing.T) {
	base := snapshot(t, "base-tree", map[string][]byte{
		"SKILL.md": []byte("---\nname: sample\n---\nBody\n"),
		"check.sh": []byte("echo base\n"),
	})
	local := localSkill(t, map[string][]byte{
		"SKILL.md": []byte(installed("Body\n")),
		"check.sh": []byte("echo local\n"),
	})
	remote := snapshot(t, "remote-tree", map[string][]byte{
		"SKILL.md": []byte("---\nname: sample\n---\nBody\n"),
		"check.sh": []byte("echo base\n"),
	})
	base.Executable["check.sh"] = false
	local.Executable["check.sh"] = false
	remote.Executable["check.sh"] = true

	merged, conflictPaths, err := ThreeWay(context.Background(), gitcli.New(command.New("git")), base, local, remote)

	if err != nil || len(conflictPaths) != 0 {
		t.Fatalf("ThreeWay() conflict paths=%v error=%v", conflictPaths, err)
	}
	if string(merged.Files["check.sh"]) != "echo local\n" || !merged.Executable["check.sh"] {
		t.Fatalf("merged file=%q mode=%v, want local content and remote executable mode", merged.Files["check.sh"], merged.Executable["check.sh"])
	}
}

func TestThreeWayTreatsModeChangeVersusDeleteAsConflict(t *testing.T) {
	base := snapshot(t, "base-tree", map[string][]byte{
		"SKILL.md": []byte("---\nname: sample\n---\nBody\n"),
		"check.sh": []byte("echo base\n"),
	})
	local := localSkill(t, map[string][]byte{"SKILL.md": []byte(installed("Body\n"))})
	remote := snapshot(t, "remote-tree", map[string][]byte{
		"SKILL.md": []byte("---\nname: sample\n---\nBody\n"),
		"check.sh": []byte("echo base\n"),
	})
	remote.Executable["check.sh"] = true

	_, _, err := ThreeWay(context.Background(), gitcli.New(command.New("git")), base, local, remote)

	if !errors.Is(err, ErrUnsupportedConflict) {
		t.Fatalf("ThreeWay() error=%v, want modify/delete conflict", err)
	}
}

func TestThreeWayWritesMarkersForSkillFrontmatterConflict(t *testing.T) {
	base := snapshot(t, "base-tree", map[string][]byte{"SKILL.md": []byte("---\nname: base\n---\n\nBody\n")})
	localFiles := map[string][]byte{"SKILL.md": []byte(installedWithName("local", "Body\n"))}
	local := localSkill(t, localFiles)
	remote := snapshot(t, "remote-tree", map[string][]byte{"SKILL.md": []byte("---\nname: remote\n---\n\nBody\n")})

	merged, conflictPaths, err := ThreeWay(context.Background(), gitcli.New(command.New("git")), base, local, remote)

	if err != nil || len(conflictPaths) != 1 || conflictPaths[0] != "SKILL.md" || !bytes.Contains(merged.Files["SKILL.md"], []byte("<<<<<<< gh-linked-skills:local")) {
		t.Fatalf("ThreeWay() merged=%q conflict paths=%v error=%v, want markers", merged.Files["SKILL.md"], conflictPaths, err)
	}
}

func snapshot(t *testing.T, treeSHA string, files map[string][]byte) source.SkillSnapshot {
	t.Helper()
	return remoteSnapshotForMerge(t, treeSHA, files)
}

func localSkill(t *testing.T, files map[string][]byte) workspace.LocalSkill {
	t.Helper()
	result := workspace.LocalSkill{
		Files:      files,
		Executable: map[string]bool{},
		Snapshot:   comparisonForMerge(t, files),
	}
	return result
}

func remoteSnapshotForMerge(t *testing.T, treeSHA string, files map[string][]byte) source.SkillSnapshot {
	t.Helper()
	return source.SkillSnapshot{
		TreeSHA:    treeSHA,
		Files:      files,
		Executable: map[string]bool{},
	}
}

func comparisonForMerge(t *testing.T, files map[string][]byte) map[string][]byte {
	t.Helper()
	comparison := make(map[string][]byte, len(files))
	for path, content := range files {
		comparison[path] = content
	}
	return comparison
}

func installed(body string) string {
	return installedWithName("sample", body)
}

func installedWithName(name, body string) string {
	return "---\nname: " + name + "\n---\n" + body
}
