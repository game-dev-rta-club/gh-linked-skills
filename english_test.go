package main

import (
	"bytes"
	"os"
	"os/exec"
	"testing"
	"unicode"
	"unicode/utf8"
)

func TestProjectUsesEnglishScripts(t *testing.T) {
	output, err := exec.Command("git", "ls-files", "-z").Output()
	if err != nil {
		t.Fatal(err)
	}
	for _, rawPath := range bytes.Split(output, []byte{0}) {
		if len(rawPath) == 0 {
			continue
		}
		path := string(rawPath)
		if containsJapaneseText(path) {
			t.Errorf("path contains a Japanese character: %s", path)
		}

		info, err := os.Lstat(path)
		if err != nil {
			t.Fatal(err)
		}
		if info.Mode()&os.ModeSymlink != 0 {
			target, err := os.Readlink(path)
			if err != nil {
				t.Fatal(err)
			}
			if containsJapaneseText(target) {
				t.Errorf("symlink target contains a Japanese character: %s", path)
			}
			continue
		}

		content, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if utf8.Valid(content) && containsJapaneseText(string(content)) {
			t.Errorf("file contains a Japanese character: %s", path)
		}
	}
}

func containsJapaneseText(value string) bool {
	for _, character := range value {
		if unicode.In(character, unicode.Han, unicode.Hiragana, unicode.Katakana) ||
			character >= 0x3000 && character <= 0x303f ||
			character >= 0xff00 && character <= 0xffef {
			return true
		}
	}
	return false
}

func TestContainsJapaneseText(t *testing.T) {
	for _, value := range []string{"\u3042", "\u30a2", "\u6f22", "\u3001", "\uff21"} {
		if !containsJapaneseText(value) {
			t.Errorf("containsJapaneseText(%q) = false, want true", value)
		}
	}
	if containsJapaneseText("Agent Skills") {
		t.Error("containsJapaneseText() rejected English text")
	}
}

func TestSymlinkTargetIsCheckedWithoutFollowingIt(t *testing.T) {
	link := t.TempDir() + "/linked-skill"
	if err := os.Symlink("\u3042", link); err != nil {
		t.Fatal(err)
	}
	target, err := os.Readlink(link)
	if err != nil {
		t.Fatal(err)
	}
	if !containsJapaneseText(target) {
		t.Errorf("containsJapaneseText(%q) = false, want true", target)
	}
}

func TestCurrentProjectUsesSkillLinkerIdentity(t *testing.T) {
	output, err := exec.Command("git", "ls-files", "-z").Output()
	if err != nil {
		t.Fatal(err)
	}
	allowedLegacyReferences := map[string]bool{
		"CHANGELOG.md":                      true,
		"docs/migration-to-skill-linker.md": true,
		"internal/cli/run_test.go":          true,
	}
	legacyIdentifiers := [][]byte{
		[]byte("gh-" + "linked-skills"),
		[]byte("gh " + "linked-skills"),
		[]byte("Linked " + "Skills"),
	}
	for _, rawPath := range bytes.Split(output, []byte{0}) {
		if len(rawPath) == 0 {
			continue
		}
		path := string(rawPath)
		if allowedLegacyReferences[path] {
			continue
		}
		for _, identifier := range legacyIdentifiers {
			if bytes.Contains(rawPath, identifier) {
				t.Errorf("tracked path contains former identity %q: %s", identifier, path)
			}
		}

		info, err := os.Lstat(path)
		if err != nil {
			t.Fatal(err)
		}
		var content []byte
		if info.Mode()&os.ModeSymlink != 0 {
			target, err := os.Readlink(path)
			if err != nil {
				t.Fatal(err)
			}
			content = []byte(target)
		} else {
			content, err = os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if !utf8.Valid(content) {
				continue
			}
		}
		for _, identifier := range legacyIdentifiers {
			if bytes.Contains(content, identifier) {
				t.Errorf("tracked file contains former identity %q: %s", identifier, path)
			}
		}
	}
}
