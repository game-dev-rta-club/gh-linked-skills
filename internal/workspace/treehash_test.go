package workspace

import "testing"

func TestTreeSHAReproducesGitTreeObject(t *testing.T) {
	files := map[string][]byte{
		"SKILL.md":        []byte("hello\n"),
		"foo.bar":         []byte("flat\n"),
		"foo/item.txt":    []byte("nested\n"),
		"scripts/run.mjs": []byte("run\n"),
	}
	executable := map[string]bool{"scripts/run.mjs": true}

	got, err := TreeSHA(files, executable)

	if err != nil {
		t.Fatal(err)
	}
	if got != "0df957353e75be8a6a8d9430587c11af2a6822c0" {
		t.Fatalf("TreeSHA() = %q, want Git tree SHA", got)
	}
}

func TestTreeSHAIncludesExecutableMode(t *testing.T) {
	files := map[string][]byte{"SKILL.md": []byte("hello\n")}

	regular, err := TreeSHA(files, map[string]bool{})
	if err != nil {
		t.Fatal(err)
	}
	executable, err := TreeSHA(files, map[string]bool{"SKILL.md": true})
	if err != nil {
		t.Fatal(err)
	}

	if regular != "456850a02c40a8f6f5c712a17f7a0af65b0e9a79" {
		t.Fatalf("regular TreeSHA() = %q", regular)
	}
	if executable != "9aefc32c123f9b7792e3d4f339ab362c1e559f23" {
		t.Fatalf("executable TreeSHA() = %q", executable)
	}
}

func TestTreeSHARejectsUnsafePaths(t *testing.T) {
	for _, path := range []string{"../SKILL.md", "/SKILL.md", "nested//file"} {
		if _, err := TreeSHA(map[string][]byte{path: []byte("x")}, nil); err == nil {
			t.Fatalf("TreeSHA(%q) error = nil", path)
		}
	}
}
