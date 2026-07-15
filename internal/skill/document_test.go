package skill

import (
	"errors"
	"strings"
	"testing"
)

func TestParseNameAcceptsCodexCompatibleSkill(t *testing.T) {
	name, err := ParseName([]byte("---\nname: sample-skill\ndescription: Example skill.\n---\nBody\n"))
	if err != nil || name != "sample-skill" {
		t.Fatalf("ParseName() = %q, %v", name, err)
	}
}

func TestParseNameRejectsInvalidDocuments(t *testing.T) {
	tests := []string{
		"no frontmatter",
		"---\nname: Bad_Name\ndescription: Example.\n---\n",
		"---\nname: sample\n---\n",
		"---\nname: sample\ndescription: Example.\n",
	}
	for _, input := range tests {
		if _, err := ParseName([]byte(input)); !errors.Is(err, ErrInvalidDocument) {
			t.Errorf("ParseName(%q) error = %v", input, err)
		}
	}
}

func TestParseNameRejectsOversizedDescription(t *testing.T) {
	input := "---\nname: sample\ndescription: " + strings.Repeat("x", 1025) + "\n---\n"
	if _, err := ParseName([]byte(input)); !errors.Is(err, ErrInvalidDocument) {
		t.Fatalf("ParseName() error = %v", err)
	}
}

func TestParseNameCountsDescriptionCharactersInsteadOfBytes(t *testing.T) {
	input := "---\nname: sample\ndescription: " + strings.Repeat("あ", 1024) + "\n---\n"

	name, err := ParseName([]byte(input))

	if err != nil || name != "sample" {
		t.Fatalf("ParseName() = %q, %v, want sample", name, err)
	}
}
