package status

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/game-dev-rta-club/gh-skill-linker/internal/manifest"
	"github.com/game-dev-rta-club/gh-skill-linker/internal/source"
	"github.com/game-dev-rta-club/gh-skill-linker/internal/syncstate"
	"github.com/game-dev-rta-club/gh-skill-linker/internal/workspace"
)

type fakeLister struct {
	skills []manifest.InstalledSkill
	err    error
	root   *string
}

func (f fakeLister) ListProject(_ context.Context, root string) ([]manifest.InstalledSkill, error) {
	if f.root != nil {
		*f.root = root
	}
	return f.skills, f.err
}

type fakeLocalReader struct {
	byPath map[string]workspace.LocalSkill
	err    error
}

func (f fakeLocalReader) Read(path string) (workspace.LocalSkill, error) {
	if f.err != nil {
		return workspace.LocalSkill{}, f.err
	}
	return f.byPath[path], nil
}

type fakeRemote struct {
	snapshots       map[string]source.SkillSnapshot
	trees           map[string]source.RepositoryTree
	resolutions     map[string]source.ResolvedRef
	write           bool
	err             error
	treeErrors      map[string]error
	permissionErr   error
	preflightCalls  *int
	resolveCalls    *int
	treeCalls       *int
	snapshotCalls   *int
	permissionCalls *int
}

func (f fakeRemote) ResolveSourceRef(_ context.Context, _ Repository, ref string) (source.ResolvedRef, error) {
	if f.resolveCalls != nil {
		(*f.resolveCalls)++
	}
	if f.err != nil {
		return source.ResolvedRef{}, f.err
	}
	if resolved, ok := f.resolutions[ref]; ok {
		return resolved, nil
	}
	return source.ResolvedRef{RefSHA: ref, CommitSHA: ref}, nil
}

func (f fakeRemote) ReadSkill(_ context.Context, _ Repository, _ string, revision string) (source.SkillSnapshot, error) {
	if f.snapshotCalls != nil {
		(*f.snapshotCalls)++
	}
	if f.err != nil {
		return source.SkillSnapshot{}, f.err
	}
	return f.snapshots[revision], nil
}

func (f fakeRemote) ReadRepositoryTree(_ context.Context, _ Repository, revision string) (source.RepositoryTree, error) {
	if f.treeCalls != nil {
		(*f.treeCalls)++
	}
	if f.err != nil {
		return source.RepositoryTree{}, f.err
	}
	if err := f.treeErrors[revision]; err != nil {
		return source.RepositoryTree{}, err
	}
	if tree, ok := f.trees[revision]; ok {
		return tree, nil
	}
	snapshot := f.snapshots[revision]
	return repositoryTree(map[string]string{"skills/sample": snapshot.TreeSHA}), nil
}

func (f fakeRemote) ReadRepositoryPermission(context.Context, Repository) (bool, error) {
	if f.permissionCalls != nil {
		(*f.permissionCalls)++
	}
	if f.err != nil {
		return false, f.err
	}
	if f.permissionErr != nil {
		return false, f.permissionErr
	}
	return f.write, nil
}

func (f fakeRemote) ReadStatusPreflight(ctx context.Context, requests []PreflightRequest) map[string]PreflightResult {
	if f.preflightCalls != nil {
		(*f.preflightCalls)++
	}
	results := make(map[string]PreflightResult, len(requests))
	for _, request := range requests {
		result := PreflightResult{Refs: make(map[string]PreflightRefResult)}
		for _, ref := range request.Refs {
			resolved, err := f.ResolveSourceRef(ctx, request.Repository, ref)
			result.Refs[ref] = PreflightRefResult{Resolved: resolved, Err: err}
		}
		if request.ReadPermission {
			result.PermissionChecked = true
			result.CanPush, result.PermissionErr = f.ReadRepositoryPermission(ctx, request.Repository)
		}
		results[repositoryKey(request.Repository)] = result
	}
	return results
}

func TestInspectUsesOneBatchedPreflight(t *testing.T) {
	first := managed("first", "/repo/.agents/skills/first")
	firstLocal := namedLocal(first.Name, "same\n", false)
	setBaseline(&first, firstLocal)
	second := managed("second", "/repo/.agents/skills/second")
	second.Repository = "https://github.com/other/repo.git"
	secondLocal := namedLocal(second.Name, "same\n", false)
	setBaseline(&second, secondLocal)
	preflightCalls := 0
	remote := fakeRemote{
		resolutions: map[string]source.ResolvedRef{
			"refs/heads/main": {RefSHA: "commit", CommitSHA: "commit"},
		},
		trees: map[string]source.RepositoryTree{"commit": repositoryTree(map[string]string{
			first.SourcePath: first.TreeSHA, second.SourcePath: second.TreeSHA,
		})},
		write:          true,
		preflightCalls: &preflightCalls,
	}
	reader := fakeLocalReader{byPath: map[string]workspace.LocalSkill{
		first.Path: firstLocal, second.Path: secondLocal,
	}}

	_, err := NewService(
		fakeLister{skills: []manifest.InstalledSkill{first, second}}, reader, remote,
	).Inspect(context.Background(), "/repo")

	if err != nil {
		t.Fatal(err)
	}
	if preflightCalls != 1 {
		t.Fatalf("preflight calls = %d, want 1", preflightCalls)
	}
}

func TestInspectSharesRemoteAndPermissionLookups(t *testing.T) {
	first := managed("first", "/repo/.agents/skills/first")
	firstLocal := namedLocal("first", "same\n", false)
	setBaseline(&first, firstLocal)
	second := managed("second", "/repo/.agents/skills/second")
	secondLocal := namedLocal("second", "same\n", false)
	setBaseline(&second, secondLocal)
	resolveCalls, treeCalls, snapshotCalls, permissionCalls := 0, 0, 0, 0
	remote := fakeRemote{
		resolutions: map[string]source.ResolvedRef{
			"refs/heads/main": {RefSHA: "commit", CommitSHA: "commit"},
		},
		trees: map[string]source.RepositoryTree{
			"commit": repositoryTree(map[string]string{
				first.SourcePath:  first.TreeSHA,
				second.SourcePath: second.TreeSHA,
			}),
		},
		write:           true,
		resolveCalls:    &resolveCalls,
		treeCalls:       &treeCalls,
		snapshotCalls:   &snapshotCalls,
		permissionCalls: &permissionCalls,
	}
	reader := fakeLocalReader{byPath: map[string]workspace.LocalSkill{
		first.Path: firstLocal, second.Path: secondLocal,
	}}

	records, err := NewService(fakeLister{skills: []manifest.InstalledSkill{first, second}}, reader, remote).Inspect(context.Background(), "/repo")

	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 2 || *records[0].State != syncstate.Clean || *records[1].State != syncstate.Clean {
		t.Fatalf("records = %#v", records)
	}
	if resolveCalls != 1 || treeCalls != 1 || permissionCalls != 1 || snapshotCalls != 0 {
		t.Fatalf("calls: resolve=%d tree=%d permission=%d snapshot=%d", resolveCalls, treeCalls, permissionCalls, snapshotCalls)
	}
}

func TestInspectSkipsRepositoryTreeWhenCommitIsUnchanged(t *testing.T) {
	entry := managed("sample", "/repo/.agents/skills/sample")
	entry.CommitSHA = "current-commit"
	treeCalls, snapshotCalls := 0, 0
	remote := fakeRemote{
		resolutions: map[string]source.ResolvedRef{
			entry.SourceRef: {RefSHA: entry.CommitSHA, CommitSHA: entry.CommitSHA},
		},
		treeErrors:    map[string]error{entry.CommitSHA: errors.New("tree must not be read")},
		write:         true,
		treeCalls:     &treeCalls,
		snapshotCalls: &snapshotCalls,
	}
	reader := fakeLocalReader{byPath: map[string]workspace.LocalSkill{entry.Path: local("same\n", false)}}

	records, err := NewService(fakeLister{skills: []manifest.InstalledSkill{entry}}, reader, remote).Inspect(context.Background(), "/repo")

	if err != nil {
		t.Fatal(err)
	}
	if records[0].State == nil || *records[0].State != syncstate.Clean {
		t.Fatalf("record = %#v", records[0])
	}
	if treeCalls != 0 || snapshotCalls != 0 {
		t.Fatalf("calls: tree=%d snapshot=%d", treeCalls, snapshotCalls)
	}
}

func TestInspectDetectsMovedTagWithoutTreeWhenCommitIsUnchanged(t *testing.T) {
	entry := managed("sample", "/repo/.agents/skills/sample")
	entry.SourceRef = "refs/tags/v1.0.0"
	entry.RefSHA = "old-tag-object"
	entry.CommitSHA = "same-commit"
	treeCalls := 0
	remote := fakeRemote{
		resolutions: map[string]source.ResolvedRef{
			entry.SourceRef: {RefSHA: "new-tag-object", CommitSHA: entry.CommitSHA},
		},
		treeErrors: map[string]error{entry.CommitSHA: errors.New("tree must not be read")},
		treeCalls:  &treeCalls,
	}
	reader := fakeLocalReader{byPath: map[string]workspace.LocalSkill{entry.Path: local("same\n", false)}}

	records, err := NewService(fakeLister{skills: []manifest.InstalledSkill{entry}}, reader, remote).Inspect(context.Background(), "/repo")

	if err != nil {
		t.Fatal(err)
	}
	if records[0].State == nil || *records[0].State != syncstate.Clean || value(records[0].PullReason) != "tag_moved" {
		t.Fatalf("record = %#v", records[0])
	}
	if treeCalls != 0 {
		t.Fatalf("tree calls = %d", treeCalls)
	}
}

func TestInspectScopesLookupsByRepositoryAndRef(t *testing.T) {
	mainSkill := managed("main-skill", "/repo/.agents/skills/main-skill")
	mainLocal := namedLocal(mainSkill.Name, "same\n", false)
	setBaseline(&mainSkill, mainLocal)
	releaseSkill := managed("release-skill", "/repo/.agents/skills/release-skill")
	releaseSkill.SourceRef = "refs/heads/release"
	releaseLocal := namedLocal(releaseSkill.Name, "same\n", false)
	setBaseline(&releaseSkill, releaseLocal)
	otherSkill := managed("other-skill", "/repo/.agents/skills/other-skill")
	otherSkill.Repository = "https://github.com/other/repo.git"
	otherLocal := namedLocal(otherSkill.Name, "same\n", false)
	setBaseline(&otherSkill, otherLocal)
	resolveCalls, treeCalls, permissionCalls := 0, 0, 0
	remote := fakeRemote{
		resolutions: map[string]source.ResolvedRef{
			"refs/heads/main":    {RefSHA: "main-commit", CommitSHA: "main-commit"},
			"refs/heads/release": {RefSHA: "release-commit", CommitSHA: "release-commit"},
		},
		trees: map[string]source.RepositoryTree{
			"main-commit": repositoryTree(map[string]string{
				mainSkill.SourcePath:  mainSkill.TreeSHA,
				otherSkill.SourcePath: otherSkill.TreeSHA,
			}),
			"release-commit": repositoryTree(map[string]string{releaseSkill.SourcePath: releaseSkill.TreeSHA}),
		},
		write:           true,
		resolveCalls:    &resolveCalls,
		treeCalls:       &treeCalls,
		permissionCalls: &permissionCalls,
	}
	reader := fakeLocalReader{byPath: map[string]workspace.LocalSkill{
		mainSkill.Path: mainLocal, releaseSkill.Path: releaseLocal, otherSkill.Path: otherLocal,
	}}

	records, err := NewService(
		fakeLister{skills: []manifest.InstalledSkill{mainSkill, releaseSkill, otherSkill}},
		reader,
		remote,
	).Inspect(context.Background(), "/repo")

	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 3 {
		t.Fatalf("records = %#v", records)
	}
	if resolveCalls != 3 || treeCalls != 3 || permissionCalls != 2 {
		t.Fatalf("calls: resolve=%d tree=%d permission=%d", resolveCalls, treeCalls, permissionCalls)
	}
}

func TestInspectScopesSharedTreeFailureToItsRef(t *testing.T) {
	first := managed("first", "/repo/.agents/skills/first")
	firstLocal := namedLocal(first.Name, "same\n", false)
	setBaseline(&first, firstLocal)
	second := managed("second", "/repo/.agents/skills/second")
	secondLocal := namedLocal(second.Name, "same\n", false)
	setBaseline(&second, secondLocal)
	release := managed("release", "/repo/.agents/skills/release")
	release.SourceRef = "refs/heads/release"
	releaseLocal := namedLocal(release.Name, "same\n", false)
	setBaseline(&release, releaseLocal)
	treeCalls, permissionCalls := 0, 0
	remote := fakeRemote{
		resolutions: map[string]source.ResolvedRef{
			"refs/heads/main":    {RefSHA: "main", CommitSHA: "main"},
			"refs/heads/release": {RefSHA: "release", CommitSHA: "release"},
		},
		trees: map[string]source.RepositoryTree{
			"release": repositoryTree(map[string]string{release.SourcePath: release.TreeSHA}),
		},
		treeErrors:      map[string]error{"main": errors.New("tree unavailable")},
		write:           true,
		treeCalls:       &treeCalls,
		permissionCalls: &permissionCalls,
	}
	reader := fakeLocalReader{byPath: map[string]workspace.LocalSkill{
		first.Path: firstLocal, second.Path: secondLocal, release.Path: releaseLocal,
	}}

	records, err := NewService(
		fakeLister{skills: []manifest.InstalledSkill{first, second, release}}, reader, remote,
	).Inspect(context.Background(), "/repo")

	if err != nil {
		t.Fatal(err)
	}
	byName := recordsByName(records)
	if byName["first"].PullEligibility != Unknown || byName["second"].PullEligibility != Unknown {
		t.Fatalf("failed ref records = %#v", records)
	}
	if byName["release"].State == nil || *byName["release"].State != syncstate.Clean {
		t.Fatalf("release record = %#v", byName["release"])
	}
	if treeCalls != 2 || permissionCalls != 1 {
		t.Fatalf("calls: tree=%d permission=%d", treeCalls, permissionCalls)
	}
}

func TestInspectSharesPermissionFailureAndPreservesState(t *testing.T) {
	first := managed("first", "/repo/.agents/skills/first")
	firstLocal := namedLocal(first.Name, "same\n", false)
	setBaseline(&first, firstLocal)
	second := managed("second", "/repo/.agents/skills/second")
	secondLocal := namedLocal(second.Name, "same\n", false)
	setBaseline(&second, secondLocal)
	permissionCalls := 0
	remote := fakeRemote{
		resolutions: map[string]source.ResolvedRef{
			"refs/heads/main": {RefSHA: "commit", CommitSHA: "commit"},
		},
		trees: map[string]source.RepositoryTree{
			"commit": repositoryTree(map[string]string{
				first.SourcePath: first.TreeSHA, second.SourcePath: second.TreeSHA,
			}),
		},
		permissionErr:   errors.New("permission unavailable"),
		permissionCalls: &permissionCalls,
	}
	reader := fakeLocalReader{byPath: map[string]workspace.LocalSkill{
		first.Path: firstLocal, second.Path: secondLocal,
	}}

	records, err := NewService(
		fakeLister{skills: []manifest.InstalledSkill{first, second}}, reader, remote,
	).Inspect(context.Background(), "/repo")

	if err != nil {
		t.Fatal(err)
	}
	for _, record := range records {
		if record.State == nil || *record.State != syncstate.Clean || record.PushEligibility != Unknown || value(record.PushReason) != "permission_unknown" {
			t.Fatalf("record = %#v", record)
		}
	}
	if permissionCalls != 1 {
		t.Fatalf("permission calls = %d, want 1", permissionCalls)
	}
}

func TestInspectReportsTagAsFixedReadOnlySource(t *testing.T) {
	entry := managed("sample", "/repo/.agents/skills/sample")
	entry.SourceRef = "refs/tags/v1.0.0"
	entry.RefSHA = strings.Repeat("c", 40)
	entry.CommitSHA = strings.Repeat("a", 40)
	base := snapshot("same\n", false, entry.TreeSHA)
	base.CommitSHA = entry.CommitSHA
	reader := fakeLocalReader{byPath: map[string]workspace.LocalSkill{entry.Path: local("same\n", false)}}
	permissionCalls := 0
	remote := fakeRemote{
		snapshots: map[string]source.SkillSnapshot{entry.TreeSHA: base, entry.CommitSHA: base},
		resolutions: map[string]source.ResolvedRef{
			entry.SourceRef: {RefSHA: entry.RefSHA, CommitSHA: entry.CommitSHA},
		},
		write: true, permissionCalls: &permissionCalls,
	}

	records, err := NewService(fakeLister{skills: []manifest.InstalledSkill{entry}}, reader, remote).Inspect(context.Background(), "/repo")

	if err != nil {
		t.Fatal(err)
	}
	record := records[0]
	if record.State == nil || *record.State != syncstate.Clean ||
		record.PullEligibility != Ineligible || value(record.PullReason) != "fixed_source_ref" ||
		record.PushEligibility != Ineligible || value(record.PushReason) != "source_ref_read_only" {
		t.Fatalf("record = %#v", record)
	}
	if permissionCalls != 0 {
		t.Fatalf("permission calls = %d, want 0", permissionCalls)
	}
}

func TestInspectReportsMovedTagWithoutApplyingIt(t *testing.T) {
	entry := managed("sample", "/repo/.agents/skills/sample")
	entry.SourceRef = "refs/tags/v1.0.0"
	entry.RefSHA = strings.Repeat("c", 40)
	entry.CommitSHA = strings.Repeat("a", 40)
	base := snapshot("same\n", false, entry.TreeSHA)
	base.CommitSHA = entry.CommitSHA
	current := snapshot("remote\n", false, strings.Repeat("f", 40))
	current.CommitSHA = strings.Repeat("e", 40)
	reader := fakeLocalReader{byPath: map[string]workspace.LocalSkill{entry.Path: local("same\n", false)}}
	remote := fakeRemote{
		snapshots: map[string]source.SkillSnapshot{entry.TreeSHA: base, current.CommitSHA: current},
		resolutions: map[string]source.ResolvedRef{
			entry.SourceRef: {RefSHA: strings.Repeat("d", 40), CommitSHA: current.CommitSHA},
		},
	}

	records, err := NewService(fakeLister{skills: []manifest.InstalledSkill{entry}}, reader, remote).Inspect(context.Background(), "/repo")

	if err != nil {
		t.Fatal(err)
	}
	record := records[0]
	if record.State == nil || *record.State != syncstate.Pull || value(record.PullReason) != "tag_moved" ||
		value(record.PushReason) != "source_ref_read_only" {
		t.Fatalf("record = %#v", record)
	}
}

func TestInspectReportsMovedTagWithUnchangedTreeAsClean(t *testing.T) {
	entry := managed("sample", "/repo/.agents/skills/sample")
	entry.SourceRef = "refs/tags/v1.0.0"
	entry.RefSHA = strings.Repeat("c", 40)
	entry.CommitSHA = strings.Repeat("a", 40)
	current := snapshot("same\n", false, entry.TreeSHA)
	current.CommitSHA = strings.Repeat("e", 40)
	reader := fakeLocalReader{byPath: map[string]workspace.LocalSkill{entry.Path: local("same\n", false)}}
	remote := fakeRemote{
		snapshots: map[string]source.SkillSnapshot{current.CommitSHA: current},
		resolutions: map[string]source.ResolvedRef{
			entry.SourceRef: {RefSHA: strings.Repeat("d", 40), CommitSHA: current.CommitSHA},
		},
	}

	records, err := NewService(fakeLister{skills: []manifest.InstalledSkill{entry}}, reader, remote).Inspect(context.Background(), "/repo")

	if err != nil {
		t.Fatal(err)
	}
	if records[0].State == nil || *records[0].State != syncstate.Clean || value(records[0].PullReason) != "tag_moved" {
		t.Fatalf("record = %#v", records[0])
	}
}

type fakeInventory struct {
	tracked      []string
	push         []string
	trackedCalls *int
	pushCalls    *int
	paths        *[]string
}

func (f fakeInventory) TrackedFiles(_ context.Context, _ string, path string) ([]string, error) {
	if f.trackedCalls != nil {
		(*f.trackedCalls)++
	}
	if f.paths != nil {
		*f.paths = append(*f.paths, path)
	}
	return f.tracked, nil
}

func (f fakeInventory) PushFiles(_ context.Context, _ string, path string) ([]string, error) {
	if f.pushCalls != nil {
		(*f.pushCalls)++
	}
	if f.paths != nil {
		*f.paths = append(*f.paths, path)
	}
	return f.push, nil
}

func TestInspectReadsGitInventoryOnceForAllSkills(t *testing.T) {
	first := managed("first", "/repo/.agents/skills/first")
	firstLocal := namedLocal(first.Name, "same\n", false)
	setBaseline(&first, firstLocal)
	second := managed("second", "/repo/.agents/skills/second")
	secondLocal := namedLocal(second.Name, "same\n", false)
	setBaseline(&second, secondLocal)
	remote := fakeRemote{
		resolutions: map[string]source.ResolvedRef{"refs/heads/main": {RefSHA: "commit", CommitSHA: "commit"}},
		trees: map[string]source.RepositoryTree{"commit": repositoryTree(map[string]string{
			first.SourcePath: first.TreeSHA, second.SourcePath: second.TreeSHA,
		})},
		write: true,
	}
	reader := fakeLocalReader{byPath: map[string]workspace.LocalSkill{
		first.Path: firstLocal, second.Path: secondLocal,
	}}
	trackedCalls, pushCalls := 0, 0
	paths := []string{}
	inventory := fakeInventory{
		tracked: []string{
			".agents/skills/first/SKILL.md", ".agents/skills/second/SKILL.md",
		},
		push: []string{
			".agents/skills/first/SKILL.md", ".agents/skills/second/SKILL.md",
		},
		trackedCalls: &trackedCalls,
		pushCalls:    &pushCalls,
		paths:        &paths,
	}

	_, err := NewService(
		fakeLister{skills: []manifest.InstalledSkill{first, second}}, reader, remote, inventory,
	).Inspect(context.Background(), "/repo")

	if err != nil {
		t.Fatal(err)
	}
	if trackedCalls != 1 || pushCalls != 1 {
		t.Fatalf("inventory calls: tracked=%d push=%d", trackedCalls, pushCalls)
	}
	if len(paths) != 2 || paths[0] != ".agents/skills" || paths[1] != ".agents/skills" {
		t.Fatalf("inventory paths = %#v", paths)
	}
}

func TestInspectUsesManifestAsAuthority(t *testing.T) {
	var listedRoot string
	entry := managed("sample", "/repo/.agents/skills/sample")
	setBaseline(&entry, local("exact\n", false))
	base := snapshot("exact\n", false, entry.TreeSHA)
	lister := fakeLister{skills: []manifest.InstalledSkill{entry}, root: &listedRoot}
	reader := fakeLocalReader{byPath: map[string]workspace.LocalSkill{entry.Path: local("exact\n", false)}}
	remote := fakeRemote{snapshots: map[string]source.SkillSnapshot{
		entry.TreeSHA: base, "refs/heads/main": base,
	}, write: true}

	records, err := NewService(lister, reader, remote).Inspect(context.Background(), "/repo")

	if err != nil {
		t.Fatalf("Inspect() error = %v", err)
	}
	if listedRoot != "/repo" {
		t.Fatalf("ListProject root = %q, want /repo", listedRoot)
	}
	if len(records) != 1 || records[0].State == nil || *records[0].State != syncstate.Clean {
		t.Fatalf("records = %#v, want one clean record", records)
	}
	if value(records[0].SourceURL) != entry.Repository || value(records[0].SourceRef) != "refs/heads/main" {
		t.Fatalf("source = %v %v, want manifest source", records[0].SourceURL, records[0].SourceRef)
	}
}

func TestInspectReportsRawByteDifferenceAsPush(t *testing.T) {
	entry := managed("sample", "/repo/.agents/skills/sample")
	setBaseline(&entry, local("name: sample\n", false))
	base := snapshot("name: sample\n", false, entry.TreeSHA)
	reader := fakeLocalReader{byPath: map[string]workspace.LocalSkill{entry.Path: local("name:  sample\n", false)}}
	remote := fakeRemote{snapshots: map[string]source.SkillSnapshot{
		entry.TreeSHA: base, "refs/heads/main": base,
	}, write: true}

	records, err := NewService(fakeLister{skills: []manifest.InstalledSkill{entry}}, reader, remote).Inspect(context.Background(), "/repo")

	if err != nil {
		t.Fatal(err)
	}
	if records[0].State == nil || *records[0].State != syncstate.Push {
		t.Fatalf("state = %v, want push", records[0].State)
	}
}

func TestInspectReportsExecutableModeOnlyLocalChangeAsPush(t *testing.T) {
	entry := managed("sample", "/repo/.agents/skills/sample")
	base := snapshot("same\n", false, entry.TreeSHA)
	reader := fakeLocalReader{byPath: map[string]workspace.LocalSkill{entry.Path: local("same\n", true)}}
	remote := fakeRemote{snapshots: map[string]source.SkillSnapshot{
		entry.TreeSHA: base, "refs/heads/main": base,
	}, write: true}

	records, err := NewService(fakeLister{skills: []manifest.InstalledSkill{entry}}, reader, remote).Inspect(context.Background(), "/repo")

	if err != nil {
		t.Fatal(err)
	}
	if records[0].State == nil || *records[0].State != syncstate.Push {
		t.Fatalf("state = %v, want push", records[0].State)
	}
}

func TestInspectRequiresPullWhenTreeSHAChangesDespiteEqualContent(t *testing.T) {
	entry := managed("sample", "/repo/.agents/skills/sample")
	base := snapshot("same\n", false, entry.TreeSHA)
	current := snapshot("same\n", false, "remote-tree")
	reader := fakeLocalReader{byPath: map[string]workspace.LocalSkill{entry.Path: local("same\n", false)}}
	remote := fakeRemote{snapshots: map[string]source.SkillSnapshot{
		entry.TreeSHA: base, "refs/heads/main": current,
	}, write: true}

	records, err := NewService(fakeLister{skills: []manifest.InstalledSkill{entry}}, reader, remote).Inspect(context.Background(), "/repo")

	if err != nil {
		t.Fatal(err)
	}
	if records[0].State == nil || *records[0].State != syncstate.Pull {
		t.Fatalf("state = %v, want pull", records[0].State)
	}
	if records[0].PushEligibility != Ineligible || value(records[0].PushReason) != "remote_changed" {
		t.Fatalf("push = %q reason %v, want remote_changed", records[0].PushEligibility, records[0].PushReason)
	}
}

func TestInspectReportsConflictWhenLocalAndTreeSHAChange(t *testing.T) {
	entry := managed("sample", "/repo/.agents/skills/sample")
	base := snapshot("same\n", false, entry.TreeSHA)
	current := snapshot("same\n", false, "remote-tree")
	reader := fakeLocalReader{byPath: map[string]workspace.LocalSkill{entry.Path: local("local\n", false)}}
	remote := fakeRemote{snapshots: map[string]source.SkillSnapshot{
		entry.TreeSHA: base, "refs/heads/main": current,
	}, write: true}

	records, err := NewService(fakeLister{skills: []manifest.InstalledSkill{entry}}, reader, remote).Inspect(context.Background(), "/repo")

	if err != nil {
		t.Fatal(err)
	}
	if records[0].State == nil || *records[0].State != syncstate.Conflict {
		t.Fatalf("state = %v, want conflict", records[0].State)
	}
}

func TestInspectReportsReadOnlyRepository(t *testing.T) {
	entry := managed("sample", "/repo/.agents/skills/sample")
	base := snapshot("same\n", false, entry.TreeSHA)
	reader := fakeLocalReader{byPath: map[string]workspace.LocalSkill{entry.Path: local("same\n", false)}}
	remote := fakeRemote{snapshots: map[string]source.SkillSnapshot{
		entry.TreeSHA: base, "refs/heads/main": base,
	}}

	records, err := NewService(fakeLister{skills: []manifest.InstalledSkill{entry}}, reader, remote).Inspect(context.Background(), "/repo")

	if err != nil {
		t.Fatal(err)
	}
	if records[0].PullEligibility != Eligible || records[0].PushEligibility != Ineligible || value(records[0].PushReason) != "repository_read_only" {
		t.Fatalf("record = %#v, want pull eligible and push read-only", records[0])
	}
}

func TestInspectReportsInvalidLocalDocumentAsPushIneligible(t *testing.T) {
	entry := managed("sample", "/repo/.agents/skills/sample")
	base := snapshot("same\n", false, entry.TreeSHA)
	invalid := []byte("---\nname: other\ndescription: Other skill.\n---\nlocal change\n")
	reader := fakeLocalReader{byPath: map[string]workspace.LocalSkill{
		entry.Path: {
			Files:      map[string][]byte{"SKILL.md": invalid},
			Executable: map[string]bool{"SKILL.md": false},
			Snapshot:   syncstate.Snapshot{"SKILL.md": invalid},
		},
	}}
	remote := fakeRemote{snapshots: map[string]source.SkillSnapshot{
		entry.TreeSHA: base, "refs/heads/main": base,
	}, write: true}

	records, err := NewService(fakeLister{skills: []manifest.InstalledSkill{entry}}, reader, remote).Inspect(context.Background(), "/repo")

	if err != nil {
		t.Fatal(err)
	}
	if records[0].PullEligibility != Eligible || records[0].PushEligibility != Ineligible || value(records[0].PushReason) != "invalid_local_skill" {
		t.Fatalf("record = %#v, want pull eligible and push invalid_local_skill", records[0])
	}
}

func TestInspectReportsUnknownWhenRemoteCannotBeRead(t *testing.T) {
	entry := managed("sample", "/repo/.agents/skills/sample")
	reader := fakeLocalReader{byPath: map[string]workspace.LocalSkill{entry.Path: local("same\n", false)}}

	records, err := NewService(fakeLister{skills: []manifest.InstalledSkill{entry}}, reader, fakeRemote{err: errors.New("denied")}).Inspect(context.Background(), "/repo")

	if err != nil {
		t.Fatal(err)
	}
	if records[0].State != nil || records[0].PullEligibility != Unknown || records[0].PushEligibility != Unknown {
		t.Fatalf("record = %#v, want unknown eligibility", records[0])
	}
}

func TestInspectReportsSourceUnavailableWhenRemoteNameChanges(t *testing.T) {
	entry := managed("sample", "/repo/.agents/skills/sample")
	base := snapshot("same\n", false, entry.TreeSHA)
	current := snapshot("remote\n", false, "remote-tree")
	current.Files["SKILL.md"] = []byte("---\nname: other\ndescription: Other skill.\n---\nremote\n")
	reader := fakeLocalReader{byPath: map[string]workspace.LocalSkill{entry.Path: local("same\n", false)}}
	remote := fakeRemote{snapshots: map[string]source.SkillSnapshot{
		entry.TreeSHA: base, "refs/heads/main": current,
	}, write: true}

	records, err := NewService(fakeLister{skills: []manifest.InstalledSkill{entry}}, reader, remote).Inspect(context.Background(), "/repo")

	if err != nil {
		t.Fatal(err)
	}
	if records[0].State != nil || records[0].PullEligibility != Unknown || value(records[0].PullReason) != "source_unavailable" {
		t.Fatalf("record = %#v, want source_unavailable", records[0])
	}
}

func TestInspectReportsWorkspaceSafetyEligibility(t *testing.T) {
	entry := managed("sample", "/repo/sample")
	base := snapshot("same\n", false, entry.TreeSHA)
	base.Files["new.mjs"] = []byte("new\n")
	base.Executable["new.mjs"] = false
	localSkill := workspace.LocalSkill{Files: base.Files, Executable: base.Executable, Snapshot: syncstate.Snapshot(base.Files)}
	setBaseline(&entry, localSkill)
	base.TreeSHA = entry.TreeSHA
	reader := fakeLocalReader{byPath: map[string]workspace.LocalSkill{entry.Path: localSkill}}
	remote := fakeRemote{snapshots: map[string]source.SkillSnapshot{entry.TreeSHA: base, "refs/heads/main": base}, write: true}
	inventory := fakeInventory{tracked: []string{"sample/SKILL.md"}, push: []string{"sample/SKILL.md"}}

	records, err := NewService(fakeLister{skills: []manifest.InstalledSkill{entry}}, reader, remote, inventory).Inspect(context.Background(), "/repo")

	if err != nil {
		t.Fatal(err)
	}
	if value(records[0].PullReason) != "untracked_files" || value(records[0].PushReason) != "ignored_files" {
		t.Fatalf("record = %#v, want workspace safety reasons", records[0])
	}
}

func TestInspectMakesGeneratedConflictIneligibleWithoutRemoteLookup(t *testing.T) {
	entry := managed("sample", "/repo/sample")
	marker := "||||||| gh-skill-linker:base:base-sha\n"
	reader := fakeLocalReader{byPath: map[string]workspace.LocalSkill{entry.Path: local(marker, false)}}

	records, err := NewService(fakeLister{skills: []manifest.InstalledSkill{entry}}, reader, fakeRemote{err: errors.New("must not be called")}).Inspect(context.Background(), "/repo")

	if err != nil {
		t.Fatal(err)
	}
	if records[0].State == nil || *records[0].State != syncstate.Conflict || value(records[0].PullReason) != "unresolved_conflict" || value(records[0].PushReason) != "unresolved_conflict" {
		t.Fatalf("record = %#v, want unresolved conflict", records[0])
	}
}

func TestSkillTreeSHASelectsExactSkillSubtree(t *testing.T) {
	tree := source.RepositoryTree{SHA: "root", Entries: []source.TreeEntry{
		{Path: "skills/sample", Mode: "040000", Type: "tree", SHA: "sample-tree"},
		{Path: "skills/sample/SKILL.md", Mode: "100644", Type: "blob", SHA: "document"},
		{Path: "skills/sample-extra", Mode: "040000", Type: "tree", SHA: "other-tree"},
		{Path: "skills/sample-extra/SKILL.md", Mode: "100644", Type: "blob", SHA: "other-document"},
	}}

	got, err := skillTreeSHA(tree, "skills/sample")

	if err != nil {
		t.Fatal(err)
	}
	if got != "sample-tree" {
		t.Fatalf("skillTreeSHA() = %q", got)
	}
}

func TestSkillTreeSHARejectsMissingDocumentAndUnsupportedEntry(t *testing.T) {
	tests := []source.RepositoryTree{
		{SHA: "root", Entries: []source.TreeEntry{
			{Path: "skills/sample", Mode: "040000", Type: "tree", SHA: "sample-tree"},
		}},
		{SHA: "root", Entries: []source.TreeEntry{
			{Path: "skills/sample", Mode: "040000", Type: "tree", SHA: "sample-tree"},
			{Path: "skills/sample/SKILL.md", Mode: "120000", Type: "blob", SHA: "link"},
		}},
	}

	for _, tree := range tests {
		if _, err := skillTreeSHA(tree, "skills/sample"); err == nil {
			t.Fatalf("skillTreeSHA(%#v) error = nil", tree)
		}
	}
}

func managed(name, path string) manifest.InstalledSkill {
	baseline := namedLocal(name, "same\n", false)
	treeSHA, err := workspace.TreeSHA(baseline.Files, baseline.Executable)
	if err != nil {
		panic(err)
	}
	return manifest.InstalledSkill{
		Name: name,
		Path: path,
		Skill: manifest.Skill{
			Repository:  "https://github.com/owner/repo.git",
			SourcePath:  "skills/" + name,
			SourceRef:   "refs/heads/main",
			RefSHA:      "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
			CommitSHA:   "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
			TreeSHA:     treeSHA,
			Destination: ".agents/skills/" + name,
		},
	}
}

func snapshot(content string, executable bool, treeSHA string) source.SkillSnapshot {
	document := skillDocument(content)
	return source.SkillSnapshot{
		TreeSHA:    treeSHA,
		Files:      map[string][]byte{"SKILL.md": document},
		Executable: map[string]bool{"SKILL.md": executable},
	}
}

func local(content string, executable bool) workspace.LocalSkill {
	return namedLocal("sample", content, executable)
}

func namedLocal(name, content string, executable bool) workspace.LocalSkill {
	document := namedSkillDocument(name, content)
	return workspace.LocalSkill{
		Files:      map[string][]byte{"SKILL.md": document},
		Executable: map[string]bool{"SKILL.md": executable},
		Snapshot:   syncstate.Snapshot{"SKILL.md": document},
	}
}

func skillDocument(body string) []byte {
	return namedSkillDocument("sample", body)
}

func namedSkillDocument(name, body string) []byte {
	return []byte("---\nname: " + name + "\ndescription: Example skill.\n---\n" + body)
}

func setBaseline(entry *manifest.InstalledSkill, local workspace.LocalSkill) {
	treeSHA, err := workspace.TreeSHA(local.Files, local.Executable)
	if err != nil {
		panic(err)
	}
	entry.TreeSHA = treeSHA
}

func repositoryTree(skills map[string]string) source.RepositoryTree {
	entries := make([]source.TreeEntry, 0, len(skills)*2)
	for path, treeSHA := range skills {
		entries = append(entries,
			source.TreeEntry{Path: path, Mode: "040000", Type: "tree", SHA: treeSHA},
			source.TreeEntry{Path: path + "/SKILL.md", Mode: "100644", Type: "blob", SHA: "document"},
		)
	}
	return source.RepositoryTree{SHA: "root", Entries: entries}
}

func recordsByName(records []Record) map[string]Record {
	result := make(map[string]Record, len(records))
	for _, record := range records {
		result[record.SkillName] = record
	}
	return result
}

func value(pointer *string) string {
	if pointer == nil {
		return ""
	}
	return *pointer
}
