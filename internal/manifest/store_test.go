package manifest

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestStoreListProjectReturnsManifestSkillsWithAbsolutePaths(t *testing.T) {
	root := t.TempDir()
	document := Document{
		SchemaVersion: CurrentSchemaVersion,
		Skills: map[string]Skill{
			"z-skill": validSkill("z-skill"),
			"a-skill": validSkill("a-skill"),
		},
	}
	if err := (Store{}).Write(root, document); err != nil {
		t.Fatal(err)
	}

	installed, err := (Store{}).ListProject(context.Background(), root)
	if err != nil {
		t.Fatalf("ListProject() error = %v", err)
	}
	if len(installed) != 2 || installed[0].Name != "a-skill" || installed[1].Name != "z-skill" {
		t.Fatalf("ListProject() = %#v, want sorted skills", installed)
	}
	wantPath := filepath.Join(root, ".agents", "skills", "a-skill")
	if installed[0].Path != wantPath || installed[0].Repository != document.Skills["a-skill"].Repository {
		t.Fatalf("first installed skill = %#v, want path %q and manifest data", installed[0], wantPath)
	}
}

func TestStoreAdvanceUpdatesOnlyMatchingSkillBaseline(t *testing.T) {
	root := t.TempDir()
	expected := validSkill("sample")
	document := Document{SchemaVersion: CurrentSchemaVersion, Skills: map[string]Skill{
		"sample": expected,
		"other":  validSkill("other"),
	}}
	store := Store{}
	if err := store.Write(root, document); err != nil {
		t.Fatal(err)
	}

	err := store.Advance(root, "sample", expected, strings.Repeat("c", 40), strings.Repeat("d", 40))

	if err != nil {
		t.Fatalf("Advance() error = %v", err)
	}
	updated, err := store.Read(root)
	if err != nil {
		t.Fatal(err)
	}
	if updated.Skills["sample"].CommitSHA != strings.Repeat("c", 40) || updated.Skills["sample"].TreeSHA != strings.Repeat("d", 40) {
		t.Fatalf("sample = %#v, want advanced baseline", updated.Skills["sample"])
	}
	if !reflect.DeepEqual(updated.Skills["other"], document.Skills["other"]) {
		t.Fatalf("other skill changed: %#v", updated.Skills["other"])
	}
}

func TestStoreAdvanceRejectsConcurrentEntryChange(t *testing.T) {
	root := t.TempDir()
	expected := validSkill("sample")
	changed := expected
	changed.SourceRef = "refs/heads/next"
	store := Store{}
	if err := store.Write(root, Document{SchemaVersion: CurrentSchemaVersion, Skills: map[string]Skill{"sample": changed}}); err != nil {
		t.Fatal(err)
	}

	err := store.Advance(root, "sample", expected, strings.Repeat("c", 40), strings.Repeat("d", 40))

	if !errors.Is(err, ErrManifestChanged) {
		t.Fatalf("Advance() error = %v, want ErrManifestChanged", err)
	}
}

func TestStoreReadMissingReturnsEmptyDocument(t *testing.T) {
	root := t.TempDir()

	document, err := (Store{}).Read(root)
	if err != nil {
		t.Fatalf("Read() error = %v", err)
	}
	if document.SchemaVersion != CurrentSchemaVersion {
		t.Fatalf("SchemaVersion = %d, want %d", document.SchemaVersion, CurrentSchemaVersion)
	}
	if document.Skills == nil || len(document.Skills) != 0 {
		t.Fatalf("Skills = %#v, want empty map", document.Skills)
	}
}

func TestStoreWriteReadRoundTrip(t *testing.T) {
	root := t.TempDir()
	document := Document{
		SchemaVersion: CurrentSchemaVersion,
		Skills: map[string]Skill{
			"sample": {
				Repository:  "https://github.com/nikollson/sample-skills.git",
				SourcePath:  "skills/sample",
				SourceRef:   "refs/heads/main",
				RefSHA:      strings.Repeat("a", 40),
				CommitSHA:   strings.Repeat("a", 40),
				TreeSHA:     strings.Repeat("b", 40),
				Destination: ".agents/skills/sample",
			},
		},
	}

	if err := (Store{}).Write(root, document); err != nil {
		t.Fatalf("Write() error = %v", err)
	}
	read, err := (Store{}).Read(root)
	if err != nil {
		t.Fatalf("Read() error = %v", err)
	}
	if !reflect.DeepEqual(read, document) {
		t.Fatalf("Read() = %#v, want %#v", read, document)
	}
	info, err := os.Stat(filepath.Join(root, FileName))
	if err != nil {
		t.Fatalf("Stat() error = %v", err)
	}
	if info.Mode().Perm() != 0o644 {
		t.Fatalf("manifest mode = %o, want 644", info.Mode().Perm())
	}
}

func TestStoreReadConvertsVersionOneBranchToSourceRef(t *testing.T) {
	root := t.TempDir()
	commitSHA := strings.Repeat("a", 40)
	content := `{
  "schemaVersion": 1,
  "skills": {
    "sample": {
      "repository": "https://github.com/nikollson/sample-skills.git",
      "sourcePath": "skills/sample",
      "branch": "main",
      "commitSHA": "` + commitSHA + `",
      "treeSHA": "` + strings.Repeat("b", 40) + `",
      "destination": ".agents/skills/sample"
    }
  }
}`
	if err := os.WriteFile(filepath.Join(root, FileName), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	document, err := (Store{}).Read(root)
	if err != nil {
		t.Fatalf("Read() error = %v", err)
	}
	entry := document.Skills["sample"]
	if document.SchemaVersion != 2 || entry.SourceRef != "refs/heads/main" || entry.RefSHA != commitSHA {
		t.Fatalf("converted document = %#v", document)
	}
	stored, err := os.ReadFile(filepath.Join(root, FileName))
	if err != nil {
		t.Fatal(err)
	}
	if string(stored) != content {
		t.Fatalf("Read() rewrote version one manifest: %s", stored)
	}
}

func TestStoreWritesVersionTwoSourceRefWithoutLegacyBranch(t *testing.T) {
	root := t.TempDir()
	document := Document{SchemaVersion: 2, Skills: map[string]Skill{"sample": {
		Repository:  "https://github.com/nikollson/sample-skills.git",
		SourcePath:  "skills/sample",
		SourceRef:   "refs/tags/v1.2.0",
		RefSHA:      strings.Repeat("c", 40),
		CommitSHA:   strings.Repeat("a", 40),
		TreeSHA:     strings.Repeat("b", 40),
		Destination: ".agents/skills/sample",
	}}}

	if err := (Store{}).Write(root, document); err != nil {
		t.Fatalf("Write() error = %v", err)
	}
	content, err := os.ReadFile(filepath.Join(root, FileName))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(content), `"branch"`) || !strings.Contains(string(content), `"sourceRef": "refs/tags/v1.2.0"`) {
		t.Fatalf("manifest = %s", content)
	}
}

func TestStoreRejectsInvalidDocumentsWithoutReplacingExistingManifest(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, FileName)
	original := []byte("original\n")
	if err := os.WriteFile(path, original, 0o644); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name     string
		document Document
	}{
		{name: "unknown schema", document: Document{SchemaVersion: 3, Skills: map[string]Skill{}}},
		{name: "unsafe name", document: validDocument("../sample")},
		{name: "unsafe path", document: validDocument("sample", func(skill *Skill) { skill.SourcePath = "../sample" })},
		{name: "wrong destination", document: validDocument("sample", func(skill *Skill) { skill.Destination = ".agents/skills/other" })},
		{name: "unsupported host", document: validDocument("sample", func(skill *Skill) { skill.Repository = "https://example.com/o/r.git" })},
		{name: "invalid source ref", document: validDocument("sample", func(skill *Skill) { skill.SourceRef = "refs/heads/-main" })},
		{name: "invalid ref SHA", document: validDocument("sample", func(skill *Skill) { skill.RefSHA = "not-a-sha" })},
		{name: "invalid commit", document: validDocument("sample", func(skill *Skill) { skill.CommitSHA = "not-a-sha" })},
		{name: "invalid tree", document: validDocument("sample", func(skill *Skill) { skill.TreeSHA = "not-a-sha" })},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if err := (Store{}).Write(root, test.document); err == nil {
				t.Fatal("Write() error = nil, want validation error")
			}
			got, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got, original) {
				t.Fatalf("manifest = %q, want original %q", got, original)
			}
		})
	}
}

func TestStoreReadRejectsTrailingJSON(t *testing.T) {
	root := t.TempDir()
	content := `{"schemaVersion":1,"skills":{}} {}`
	if err := os.WriteFile(filepath.Join(root, FileName), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, err := (Store{}).Read(root); err == nil {
		t.Fatal("Read() error = nil, want trailing JSON error")
	}
}

func TestStoreReadRejectsRemovedWorkflowSkillsField(t *testing.T) {
	root := t.TempDir()
	content := `{"schemaVersion":1,"skills":{},"workflowSkills":{}}`
	if err := os.WriteFile(filepath.Join(root, FileName), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, err := (Store{}).Read(root); err == nil || !strings.Contains(err.Error(), "unknown field") {
		t.Fatalf("Read() error = %v, want unknown workflowSkills field", err)
	}
}

func TestStoreAddRegistersSkillWhenManifestIsUnchanged(t *testing.T) {
	root := t.TempDir()
	store := Store{}
	expected := Document{SchemaVersion: CurrentSchemaVersion, Skills: map[string]Skill{}}

	if err := store.Add(root, "sample", expected, validSkill("sample")); err != nil {
		t.Fatal(err)
	}
	document, err := store.Read(root)
	if err != nil {
		t.Fatal(err)
	}
	if document.Skills["sample"].SourcePath != "skills/sample" {
		t.Fatalf("document = %#v", document)
	}
}

func TestStoreAddDoesNotOverwriteConcurrentManifestChange(t *testing.T) {
	root := t.TempDir()
	store := Store{}
	expected := Document{SchemaVersion: CurrentSchemaVersion, Skills: map[string]Skill{}}
	concurrent := validDocument("other", func(skill *Skill) {
		skill.SourcePath = "skills/other"
		skill.Destination = ".agents/skills/other"
	})
	if err := store.Write(root, concurrent); err != nil {
		t.Fatal(err)
	}

	err := store.Add(root, "sample", expected, validSkill("sample"))

	if !errors.Is(err, ErrManifestChanged) {
		t.Fatalf("Add() error = %v", err)
	}
	document, readErr := store.Read(root)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if len(document.Skills) != 1 || document.Skills["other"].SourcePath != "skills/other" {
		t.Fatalf("document = %#v", document)
	}
}

func validDocument(name string, mutators ...func(*Skill)) Document {
	skill := validSkill(name)
	for _, mutate := range mutators {
		mutate(&skill)
	}
	return Document{SchemaVersion: CurrentSchemaVersion, Skills: map[string]Skill{name: skill}}
}

func validSkill(name string) Skill {
	return Skill{
		Repository:  "https://github.com/nikollson/sample-skills.git",
		SourcePath:  "skills/sample",
		SourceRef:   "refs/heads/main",
		RefSHA:      strings.Repeat("a", 40),
		CommitSHA:   strings.Repeat("a", 40),
		TreeSHA:     strings.Repeat("b", 40),
		Destination: ".agents/skills/" + name,
	}
}
