package status

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/game-dev-rta-club/gh-skill-linker/internal/manifest"
	"github.com/game-dev-rta-club/gh-skill-linker/internal/proposal"
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
	pulls           []proposal.PullRequest
	pullErr         error
	pullCalls       *int
}

var fakeRemoteCounterMutex sync.Mutex

func incrementFakeRemoteCounter(counter *int) {
	if counter == nil {
		return
	}
	fakeRemoteCounterMutex.Lock()
	(*counter)++
	fakeRemoteCounterMutex.Unlock()
}

type blockingTreeRemote struct {
	fakeRemote
	commits map[string]string
	started chan string
	release chan struct{}
}

func (f *blockingTreeRemote) ReadStatusPreflight(
	_ context.Context, requests []PreflightRequest,
) map[string]PreflightResult {
	results := make(map[string]PreflightResult, len(requests))
	for _, request := range requests {
		commit := f.commits[repositoryKey(request.Repository)]
		result := PreflightResult{
			Refs:              make(map[string]PreflightRefResult, len(request.Refs)),
			CanPush:           true,
			PermissionChecked: true,
		}
		for _, ref := range request.Refs {
			result.Refs[ref] = PreflightRefResult{Resolved: source.ResolvedRef{
				RefSHA: commit, CommitSHA: commit,
			}}
		}
		results[repositoryKey(request.Repository)] = result
	}
	return results
}

func (f *blockingTreeRemote) ReadRepositoryTree(
	_ context.Context, _ Repository, revision string,
) (source.RepositoryTree, error) {
	f.started <- "tree:" + revision
	<-f.release
	return f.trees[revision], nil
}

type blockingMixedRemote struct {
	*blockingTreeRemote
}

func (f *blockingMixedRemote) ListPullRequests(
	_ context.Context, repository source.Repository, _ proposal.ListOptions,
) ([]proposal.PullRequest, error) {
	f.started <- "proposal:" + repositoryKey(repository)
	<-f.release
	return nil, nil
}

func (f fakeRemote) ListPullRequests(
	context.Context, source.Repository, proposal.ListOptions,
) ([]proposal.PullRequest, error) {
	incrementFakeRemoteCounter(f.pullCalls)
	return f.pulls, f.pullErr
}

func (f fakeRemote) ResolveSourceRef(_ context.Context, _ Repository, ref string) (source.ResolvedRef, error) {
	incrementFakeRemoteCounter(f.resolveCalls)
	if f.err != nil {
		return source.ResolvedRef{}, f.err
	}
	if resolved, ok := f.resolutions[ref]; ok {
		return resolved, nil
	}
	return source.ResolvedRef{RefSHA: ref, CommitSHA: ref}, nil
}

func (f fakeRemote) ReadSkill(_ context.Context, _ Repository, _ string, revision string) (source.SkillSnapshot, error) {
	incrementFakeRemoteCounter(f.snapshotCalls)
	if f.err != nil {
		return source.SkillSnapshot{}, f.err
	}
	return f.snapshots[revision], nil
}

func (f fakeRemote) ReadRepositoryTree(_ context.Context, _ Repository, revision string) (source.RepositoryTree, error) {
	incrementFakeRemoteCounter(f.treeCalls)
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
	incrementFakeRemoteCounter(f.permissionCalls)
	if f.err != nil {
		return false, f.err
	}
	if f.permissionErr != nil {
		return false, f.permissionErr
	}
	return f.write, nil
}

func (f fakeRemote) ReadStatusPreflight(ctx context.Context, requests []PreflightRequest) map[string]PreflightResult {
	incrementFakeRemoteCounter(f.preflightCalls)
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

func TestInspectSkipsProposalLookupForReadOnlyRepository(t *testing.T) {
	entry := managed("sample", "/repo/.agents/skills/sample")
	base := snapshot("same\n", false, entry.TreeSHA)
	pullCalls := 0
	remote := fakeRemote{
		snapshots: map[string]source.SkillSnapshot{
			entry.TreeSHA: base, "refs/heads/main": base,
		},
		pullCalls: &pullCalls,
	}

	_, err := NewService(
		fakeLister{skills: []manifest.InstalledSkill{entry}},
		fakeLocalReader{byPath: map[string]workspace.LocalSkill{entry.Path: local("same\n", false)}},
		remote,
	).Inspect(context.Background(), "/repo")

	if err != nil {
		t.Fatal(err)
	}
	if pullCalls != 0 {
		t.Fatalf("pull request lookup calls = %d, want 0 for read-only repository", pullCalls)
	}
}

func TestInspectReadsChangedRepositoryTreesConcurrently(t *testing.T) {
	first := managed("first", "/repo/.agents/skills/first")
	first.Repository = "https://github.com/owner/first.git"
	first.SourcePath = "skills/first"
	firstLocal := namedLocal(first.Name, "same\n", false)
	setBaseline(&first, firstLocal)
	second := managed("second", "/repo/.agents/skills/second")
	second.Repository = "https://github.com/owner/second.git"
	second.SourcePath = "skills/second"
	secondLocal := namedLocal(second.Name, "same\n", false)
	setBaseline(&second, secondLocal)

	release := make(chan struct{})
	remote := &blockingTreeRemote{
		fakeRemote: fakeRemote{trees: map[string]source.RepositoryTree{
			"first-current":  repositoryTree(map[string]string{first.SourcePath: first.TreeSHA}),
			"second-current": repositoryTree(map[string]string{second.SourcePath: second.TreeSHA}),
		}},
		commits: map[string]string{
			"owner/first": "first-current", "owner/second": "second-current",
		},
		started: make(chan string, 2),
		release: release,
	}
	done := make(chan error, 1)
	go func() {
		_, err := NewService(
			fakeLister{skills: []manifest.InstalledSkill{first, second}},
			fakeLocalReader{byPath: map[string]workspace.LocalSkill{
				first.Path: firstLocal, second.Path: secondLocal,
			}},
			remote,
		).Inspect(context.Background(), "/repo")
		done <- err
	}()

	for count := 0; count < 2; count++ {
		select {
		case <-remote.started:
		case <-time.After(5 * time.Second):
			close(release)
			<-done
			t.Fatal("repository tree reads did not overlap")
		}
	}
	close(release)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}

func TestInspectSharesConcurrencyLimitAcrossProposalAndTreeReads(t *testing.T) {
	const repositoryCount = 5
	skills := make([]manifest.InstalledSkill, 0, repositoryCount)
	locals := make(map[string]workspace.LocalSkill, repositoryCount)
	commits := make(map[string]string, repositoryCount)
	trees := make(map[string]source.RepositoryTree, repositoryCount)
	for index := range repositoryCount {
		name := fmt.Sprintf("sample-%d", index)
		entry := managed(name, "/repo/.agents/skills/"+name)
		entry.Repository = fmt.Sprintf("https://github.com/owner/repo-%d.git", index)
		entry.SourcePath = "skills/" + name
		if index == repositoryCount-1 {
			entry.SourceRef = "refs/tags/v1"
		}
		localSkill := namedLocal(entry.Name, "same\n", false)
		setBaseline(&entry, localSkill)
		commit := fmt.Sprintf("current-%d", index)
		repository := fmt.Sprintf("owner/repo-%d", index)
		skills = append(skills, entry)
		locals[entry.Path] = localSkill
		commits[repository] = commit
		trees[commit] = repositoryTree(map[string]string{entry.SourcePath: entry.TreeSHA})
	}

	release := make(chan struct{})
	started := make(chan string, 2*repositoryCount-1)
	remote := &blockingMixedRemote{blockingTreeRemote: &blockingTreeRemote{
		fakeRemote: fakeRemote{trees: trees},
		commits:    commits,
		started:    started,
		release:    release,
	}}
	done := make(chan error, 1)
	go func() {
		_, err := NewService(
			fakeLister{skills: skills}, fakeLocalReader{byPath: locals}, remote,
		).Inspect(context.Background(), "/repo")
		done <- err
	}()

	startedKinds := map[string]int{}
	for count := 0; count < statusRemoteConcurrency; count++ {
		select {
		case call := <-started:
			startedKinds[strings.SplitN(call, ":", 2)[0]]++
		case <-time.After(5 * time.Second):
			close(release)
			<-done
			t.Fatal("eight remote reads did not start")
		}
	}
	if startedKinds["tree"] == 0 || startedKinds["proposal"] == 0 {
		close(release)
		<-done
		t.Fatalf("started calls = %v, want both tree and proposal reads", startedKinds)
	}
	select {
	case call := <-started:
		close(release)
		<-done
		t.Fatalf("ninth remote read %q started before a shared slot was released", call)
	case <-time.After(500 * time.Millisecond):
	}
	close(release)
	if err := <-done; err != nil {
		t.Fatal(err)
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

func TestInspectShowsOpenProposalSeparatelyFromFileState(t *testing.T) {
	entry := managed("sample", "/repo/.agents/skills/sample")
	localSkill := local("changed\n", false)
	localTree, err := workspace.TreeSHA(localSkill.Files, localSkill.Executable)
	if err != nil {
		t.Fatal(err)
	}
	pull := statusPull(t, entry, localTree)
	remote := fakeRemote{
		resolutions: map[string]source.ResolvedRef{
			entry.SourceRef: {RefSHA: entry.CommitSHA, CommitSHA: entry.CommitSHA},
		},
		write: true, pulls: []proposal.PullRequest{pull},
	}

	records, err := NewService(
		fakeLister{skills: []manifest.InstalledSkill{entry}},
		fakeLocalReader{byPath: map[string]workspace.LocalSkill{entry.Path: localSkill}}, remote,
	).Inspect(context.Background(), "/repo")

	if err != nil {
		t.Fatal(err)
	}
	record := records[0]
	if record.State == nil || *record.State != syncstate.Push || record.Proposal == nil ||
		record.Proposal.State != proposal.Waiting || record.Proposal.Number != pull.Number {
		t.Fatalf("record = %#v", record)
	}
	if record.PushEligibility != Ineligible || value(record.PushReason) != "open_proposal" {
		t.Fatalf("push = %s (%s)", record.PushEligibility, value(record.PushReason))
	}
}

func TestInspectKeepsProposalVisibleWhenPermissionIsUnknown(t *testing.T) {
	entry := managed("sample", "/repo/.agents/skills/sample")
	localSkill := local("changed\n", false)
	localTree, err := workspace.TreeSHA(localSkill.Files, localSkill.Executable)
	if err != nil {
		t.Fatal(err)
	}
	pull := statusPull(t, entry, localTree)
	remote := fakeRemote{
		resolutions: map[string]source.ResolvedRef{
			entry.SourceRef: {RefSHA: entry.CommitSHA, CommitSHA: entry.CommitSHA},
		},
		permissionErr: errors.New("permission unavailable"),
		pulls:         []proposal.PullRequest{pull},
	}

	records, err := NewService(
		fakeLister{skills: []manifest.InstalledSkill{entry}},
		fakeLocalReader{byPath: map[string]workspace.LocalSkill{entry.Path: localSkill}},
		remote,
	).Inspect(context.Background(), "/repo")

	if err != nil {
		t.Fatal(err)
	}
	record := records[0]
	if record.Proposal == nil || record.Proposal.State != proposal.Waiting {
		t.Fatalf("proposal = %#v, want waiting", record.Proposal)
	}
	if record.PushEligibility != Unknown || value(record.PushReason) != "permission_unknown" {
		t.Fatalf("push = %q reason %v, want permission_unknown", record.PushEligibility, record.PushReason)
	}
}

func TestInspectKeepsFileStateWhenProposalLookupFails(t *testing.T) {
	entry := managed("sample", "/repo/.agents/skills/sample")
	remote := fakeRemote{
		resolutions: map[string]source.ResolvedRef{
			entry.SourceRef: {RefSHA: entry.CommitSHA, CommitSHA: entry.CommitSHA},
		},
		write: true, pullErr: errors.New("GitHub unavailable"),
	}

	records, err := NewService(
		fakeLister{skills: []manifest.InstalledSkill{entry}},
		fakeLocalReader{byPath: map[string]workspace.LocalSkill{entry.Path: local("same\n", false)}}, remote,
	).Inspect(context.Background(), "/repo")

	if err != nil {
		t.Fatal(err)
	}
	record := records[0]
	if record.State == nil || *record.State != syncstate.Clean || record.Proposal == nil ||
		record.Proposal.State != proposal.Unknown {
		t.Fatalf("record = %#v", record)
	}
	if record.PushEligibility != Unknown || value(record.PushReason) != "proposal_unknown" {
		t.Fatalf("push = %s (%s)", record.PushEligibility, value(record.PushReason))
	}
}

func TestInspectListsPullRequestsOncePerRepository(t *testing.T) {
	first := managed("first", "/repo/.agents/skills/first")
	second := managed("second", "/repo/.agents/skills/second")
	firstLocal := namedLocal("first", "same\n", false)
	secondLocal := namedLocal("second", "same\n", false)
	setBaseline(&first, firstLocal)
	setBaseline(&second, secondLocal)
	pullCalls := 0
	remote := fakeRemote{
		resolutions: map[string]source.ResolvedRef{
			first.SourceRef: {RefSHA: first.CommitSHA, CommitSHA: first.CommitSHA},
		},
		write: true, pullCalls: &pullCalls,
	}

	_, err := NewService(
		fakeLister{skills: []manifest.InstalledSkill{first, second}},
		fakeLocalReader{byPath: map[string]workspace.LocalSkill{first.Path: firstLocal, second.Path: secondLocal}},
		remote,
	).Inspect(context.Background(), "/repo")

	if err != nil {
		t.Fatal(err)
	}
	if pullCalls != 1 {
		t.Fatalf("pull request calls = %d, want 1", pullCalls)
	}
}

func TestInspectReportsPullWhenLocalAlreadyMatchesAdvancedRemote(t *testing.T) {
	entry := managed("sample", "/repo/.agents/skills/sample")
	currentLocal := local("new remote\n", false)
	currentTree, err := workspace.TreeSHA(currentLocal.Files, currentLocal.Executable)
	if err != nil {
		t.Fatal(err)
	}
	current := source.SkillSnapshot{
		CommitSHA: strings.Repeat("c", 40), TreeSHA: currentTree,
		Files: currentLocal.Files, Executable: currentLocal.Executable,
	}
	remote := fakeRemote{
		resolutions: map[string]source.ResolvedRef{
			entry.SourceRef: {RefSHA: current.CommitSHA, CommitSHA: current.CommitSHA},
		},
		trees: map[string]source.RepositoryTree{
			current.CommitSHA: repositoryTree(map[string]string{entry.SourcePath: current.TreeSHA}),
		},
		snapshots: map[string]source.SkillSnapshot{current.CommitSHA: current}, write: true,
	}

	records, err := NewService(
		fakeLister{skills: []manifest.InstalledSkill{entry}},
		fakeLocalReader{byPath: map[string]workspace.LocalSkill{entry.Path: currentLocal}}, remote,
	).Inspect(context.Background(), "/repo")

	if err != nil {
		t.Fatal(err)
	}
	if records[0].State == nil || *records[0].State != syncstate.Pull {
		t.Fatalf("state = %v, want pull", records[0].State)
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

func statusPull(t *testing.T, entry manifest.InstalledSkill, proposedTree string) proposal.PullRequest {
	t.Helper()
	body, err := proposal.SetMetadata("Synchronize skill.", proposal.Metadata{
		Version: proposal.MetadataVersion, SourcePath: entry.SourcePath, BaseRef: entry.SourceRef,
		BaseTreeSHA: entry.TreeSHA, ProposedTreeSHA: proposedTree,
		HeadCommitSHA: strings.Repeat("c", 40),
	})
	if err != nil {
		t.Fatal(err)
	}
	repository, _ := source.ParseRepository(entry.Repository)
	fullName := repository.Owner + "/" + repository.Name
	return proposal.PullRequest{
		Number: 42, URL: "https://github.com/owner/repo/pull/42", State: "open", Body: body,
		HeadRef: proposal.BranchPrefix(entry.Name, entry.SourcePath) + "/proposal",
		HeadSHA: strings.Repeat("c", 40), HeadRepository: fullName,
		BaseRef: strings.TrimPrefix(entry.SourceRef, "refs/heads/"), BaseRepository: fullName,
	}
}
