package proposal

import (
	"strings"
	"testing"
)

func TestMetadataRoundTripPreservesHumanBody(t *testing.T) {
	metadata := Metadata{
		Version:         MetadataVersion,
		SourcePath:      "skills/sample",
		BaseRef:         "refs/heads/main",
		BaseTreeSHA:     strings.Repeat("a", 40),
		ProposedTreeSHA: strings.Repeat("b", 40),
		HeadCommitSHA:   strings.Repeat("c", 40),
	}
	body := "Synchronize `sample` from a project.\n"

	encoded, err := SetMetadata(body, metadata)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := ParseMetadata(encoded)
	if err != nil {
		t.Fatal(err)
	}
	if decoded != metadata {
		t.Fatalf("metadata = %#v, want %#v", decoded, metadata)
	}
	if !strings.HasPrefix(encoded, body) {
		t.Fatalf("body = %q, want human text preserved", encoded)
	}

	updated := metadata
	updated.ProposedTreeSHA = strings.Repeat("d", 40)
	replaced, err := SetMetadata(encoded, updated)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(replaced, markerStart) != 1 {
		t.Fatalf("marker count = %d, want 1", strings.Count(replaced, markerStart))
	}
	decoded, err = ParseMetadata(replaced)
	if err != nil || decoded != updated {
		t.Fatalf("updated metadata = %#v, %v", decoded, err)
	}
}

func TestParseMetadataRejectsUnsafeOrAmbiguousMarkers(t *testing.T) {
	valid := Metadata{
		Version:         MetadataVersion,
		SourcePath:      "skills/sample",
		BaseRef:         "refs/heads/main",
		BaseTreeSHA:     strings.Repeat("a", 40),
		ProposedTreeSHA: strings.Repeat("b", 40),
		HeadCommitSHA:   strings.Repeat("c", 40),
	}
	body, err := SetMetadata("proposal\n", valid)
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name string
		body string
	}{
		{name: "missing", body: "proposal only"},
		{name: "multiple", body: body + "\n" + body},
		{name: "traversal", body: strings.Replace(body, "skills/sample", "../sample", 1)},
		{name: "unknown field", body: strings.Replace(body, `"version":1`, `"version":1,"extra":true`, 1)},
		{name: "trailing json", body: strings.Replace(body, "\n"+markerEnd, "\n{}\n"+markerEnd, 1)},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := ParseMetadata(test.body); err == nil {
				t.Fatalf("ParseMetadata(%q) succeeded", test.name)
			}
		})
	}
}

func TestClassifyCurrentProposal(t *testing.T) {
	sha := func(value byte) string { return strings.Repeat(string(value), 40) }
	metadata := Metadata{
		Version: MetadataVersion, SourcePath: "skills/sample", BaseRef: "refs/heads/main",
		BaseTreeSHA: sha('a'), ProposedTreeSHA: sha('b'), HeadCommitSHA: sha('c'),
	}
	tests := []struct {
		name    string
		local   string
		base    string
		head    string
		want    State
		applied bool
	}{
		{name: "waiting", local: sha('b'), base: sha('a'), head: sha('c'), want: Waiting},
		{name: "local update", local: sha('d'), base: sha('a'), head: sha('c'), want: Update},
		{name: "local reverted", local: sha('a'), base: sha('a'), head: sha('c'), want: Obsolete},
		{name: "base changed", local: sha('b'), base: sha('d'), head: sha('c'), want: SourceChanged},
		{name: "head changed externally", local: sha('b'), base: sha('a'), head: sha('d'), want: Diverged},
		{name: "base already contains proposal", local: sha('b'), base: sha('b'), head: sha('c'), applied: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			state, applied := Classify(test.local, test.base, test.head, metadata)
			if state != test.want || applied != test.applied {
				t.Fatalf("Classify() = %q, %t; want %q, %t", state, applied, test.want, test.applied)
			}
		})
	}
}

func TestBranchPrefixIsStableAndScoped(t *testing.T) {
	first := BranchPrefix("sample", "skills/team/sample")
	second := BranchPrefix("sample", "skills/team/sample")
	other := BranchPrefix("sample", "skills/other/sample")
	if first != second || first == other {
		t.Fatalf("prefixes = %q, %q, %q", first, second, other)
	}
	if !strings.HasPrefix(first, "skill-linker/sample-") {
		t.Fatalf("prefix = %q", first)
	}
	branch := BranchName(first, strings.Repeat("a", 40), 2)
	if !strings.HasPrefix(branch, first+"/") || !strings.HasSuffix(branch, "-2") {
		t.Fatalf("branch = %q", branch)
	}
}
