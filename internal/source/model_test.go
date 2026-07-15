package source

import (
	"testing"
)

func TestParseRefRecognizesBranchAndTag(t *testing.T) {
	tests := []struct {
		value string
		kind  RefKind
		name  string
	}{
		{value: "refs/heads/main", kind: BranchRef, name: "main"},
		{value: "refs/tags/v1.2.0", kind: TagRef, name: "v1.2.0"},
	}
	for _, test := range tests {
		ref, err := ParseRef(test.value)
		if err != nil || ref.Kind != test.kind || ref.Name != test.name || ref.FullName != test.value {
			t.Fatalf("ParseRef(%q) = %#v, %v", test.value, ref, err)
		}
	}
}

func TestNewRefRejectsUnsafeNames(t *testing.T) {
	for _, name := range []string{"", "-main", "../main", "main..next", "main lock", "main@{1}"} {
		if _, err := NewRef(BranchRef, name); err == nil {
			t.Errorf("NewRef(BranchRef, %q) error = nil", name)
		}
	}
}

func TestSkillSnapshotExactUsesRawBytesAndExecutableMode(t *testing.T) {
	left := SkillSnapshot{
		Files:      map[string][]byte{"SKILL.md": []byte("name: sample\n")},
		Executable: map[string]bool{"SKILL.md": false},
	}
	rawDifference := SkillSnapshot{
		Files:      map[string][]byte{"SKILL.md": []byte("name:  sample\n")},
		Executable: map[string]bool{"SKILL.md": false},
	}
	modeDifference := SkillSnapshot{
		Files:      map[string][]byte{"SKILL.md": []byte("name: sample\n")},
		Executable: map[string]bool{"SKILL.md": true},
	}

	if left.Exact(rawDifference) {
		t.Fatal("Exact() ignored byte difference")
	}
	if left.Exact(modeDifference) {
		t.Fatal("Exact() ignored executable mode")
	}
	if !left.Exact(left) {
		t.Fatal("Exact() rejected identical snapshot")
	}
}
