package proposal

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/game-dev-rta-club/gh-skill-linker/internal/gitcli"
	"github.com/game-dev-rta-club/gh-skill-linker/internal/source"
)

type fakeRemote struct {
	open        []PullRequest
	byHead      map[string][]PullRequest
	skills      map[string]source.SkillSnapshot
	created     PullRequest
	updated     PullRequest
	listCalls   []ListOptions
	createCalls []CreateRequest
	updateCalls []struct {
		number int
		body   string
	}
}

func (fake *fakeRemote) ListPullRequests(_ context.Context, _ source.Repository, options ListOptions) ([]PullRequest, error) {
	fake.listCalls = append(fake.listCalls, options)
	if options.Head != "" {
		return append([]PullRequest(nil), fake.byHead[options.Head]...), nil
	}
	return append([]PullRequest(nil), fake.open...), nil
}

func (fake *fakeRemote) CreatePullRequest(_ context.Context, _ source.Repository, request CreateRequest) (PullRequest, error) {
	fake.createCalls = append(fake.createCalls, request)
	result := fake.created
	if result.Number == 0 {
		result = PullRequest{Number: 42, URL: "https://github.com/owner/repo/pull/42", State: "open", Body: request.Body,
			HeadRef: request.Head, HeadSHA: strings.Repeat("c", 40), HeadRepository: "owner/repo",
			BaseRef: request.Base, BaseRepository: "owner/repo"}
	}
	return result, nil
}

func (fake *fakeRemote) UpdatePullRequestBody(_ context.Context, _ source.Repository, number int, body string) (PullRequest, error) {
	fake.updateCalls = append(fake.updateCalls, struct {
		number int
		body   string
	}{number: number, body: body})
	result := fake.updated
	if result.Number == 0 {
		result = PullRequest{Number: number, URL: "https://github.com/owner/repo/pull/42", State: "open", Body: body,
			HeadRef: "skill-linker/sample", HeadSHA: strings.Repeat("d", 40), HeadRepository: "owner/repo",
			BaseRef: "main", BaseRepository: "owner/repo"}
	}
	return result, nil
}

func (fake *fakeRemote) ReadSkill(_ context.Context, _ source.Repository, _ string, revision string) (source.SkillSnapshot, error) {
	snapshot, ok := fake.skills[revision]
	if !ok {
		return source.SkillSnapshot{}, errors.New("missing fake skill")
	}
	return snapshot, nil
}

type fakeGit struct {
	refs     map[string]string
	result   gitcli.PushResult
	requests []gitcli.ProposalRequest
}

func (fake *fakeGit) FindRef(_ context.Context, _, ref string) (string, bool, error) {
	sha, ok := fake.refs[ref]
	return sha, ok, nil
}

func (fake *fakeGit) ProposeSkill(_ context.Context, request gitcli.ProposalRequest) (gitcli.PushResult, error) {
	fake.requests = append(fake.requests, request)
	return fake.result, nil
}

func TestServiceCreatesNewProposal(t *testing.T) {
	local := snapshotWithTree('b')
	remote := &fakeRemote{}
	git := &fakeGit{refs: map[string]string{}, result: gitcli.PushResult{
		CommitSHA: strings.Repeat("c", 40), TreeSHA: local.TreeSHA, Pushed: true,
	}}
	service := NewService(remote, git)

	result, err := service.Propose(context.Background(), proposalRequest(local, strings.Repeat("a", 40)))
	if err != nil {
		t.Fatal(err)
	}
	if !result.Created || result.PullRequest.Number != 42 || len(git.requests) != 1 || len(remote.createCalls) != 1 {
		t.Fatalf("result=%#v git=%d create=%d", result, len(git.requests), len(remote.createCalls))
	}
	metadata, err := ParseMetadata(remote.createCalls[0].Body)
	if err != nil {
		t.Fatal(err)
	}
	if metadata.BaseTreeSHA != strings.Repeat("a", 40) || metadata.ProposedTreeSHA != local.TreeSHA ||
		metadata.HeadCommitSHA != strings.Repeat("c", 40) {
		t.Fatalf("metadata = %#v", metadata)
	}
}

func TestServiceReturnsWaitingForCurrentOpenProposal(t *testing.T) {
	local := snapshotWithTree('b')
	remote := &fakeRemote{open: []PullRequest{openPull(t, 'a', 'b', 'c')}}
	git := &fakeGit{}

	result, err := NewService(remote, git).Propose(
		context.Background(), proposalRequest(local, strings.Repeat("a", 40)),
	)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Waiting || len(git.requests) != 0 || len(remote.updateCalls) != 0 {
		t.Fatalf("result=%#v git=%d update=%d", result, len(git.requests), len(remote.updateCalls))
	}
}

func TestServiceUpdatesSameProposalForAdditionalLocalChange(t *testing.T) {
	local := snapshotWithTree('d')
	remote := &fakeRemote{open: []PullRequest{openPull(t, 'a', 'b', 'c')}}
	git := &fakeGit{result: gitcli.PushResult{CommitSHA: strings.Repeat("e", 40), TreeSHA: local.TreeSHA, Pushed: true}}

	result, err := NewService(remote, git).Propose(
		context.Background(), proposalRequest(local, strings.Repeat("a", 40)),
	)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Updated || len(git.requests) != 1 || git.requests[0].MergeBase || len(remote.updateCalls) != 1 {
		t.Fatalf("result=%#v git=%#v update=%d", result, git.requests, len(remote.updateCalls))
	}
	metadata, err := ParseMetadata(remote.updateCalls[0].body)
	if err != nil || metadata.ProposedTreeSHA != local.TreeSHA || metadata.HeadCommitSHA != strings.Repeat("e", 40) {
		t.Fatalf("metadata=%#v error=%v", metadata, err)
	}
}

func TestServiceMergesCurrentBaseAfterCallerReconciles(t *testing.T) {
	local := snapshotWithTree('e')
	remote := &fakeRemote{open: []PullRequest{openPull(t, 'a', 'b', 'c')}}
	git := &fakeGit{result: gitcli.PushResult{CommitSHA: strings.Repeat("f", 40), TreeSHA: local.TreeSHA, Pushed: true}}

	result, err := NewService(remote, git).Propose(
		context.Background(), proposalRequest(local, strings.Repeat("d", 40)),
	)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Updated || len(git.requests) != 1 || !git.requests[0].MergeBase {
		t.Fatalf("result=%#v request=%#v", result, git.requests)
	}
}

func TestServiceRecoversPushThatSucceededBeforeMetadataUpdate(t *testing.T) {
	local := snapshotWithTree('b')
	pull := openPull(t, 'a', 'b', 'c')
	pull.HeadSHA = strings.Repeat("d", 40)
	remote := &fakeRemote{
		open:   []PullRequest{pull},
		skills: map[string]source.SkillSnapshot{pull.HeadSHA: local},
	}
	git := &fakeGit{}

	result, err := NewService(remote, git).Propose(
		context.Background(), proposalRequest(local, strings.Repeat("a", 40)),
	)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Recovered || len(git.requests) != 0 || len(remote.updateCalls) != 1 {
		t.Fatalf("result=%#v git=%d update=%d", result, len(git.requests), len(remote.updateCalls))
	}
}

func TestServiceRefusesExternallyChangedProposal(t *testing.T) {
	local := snapshotWithTree('b')
	pull := openPull(t, 'a', 'b', 'c')
	pull.HeadSHA = strings.Repeat("d", 40)
	remote := &fakeRemote{
		open:   []PullRequest{pull},
		skills: map[string]source.SkillSnapshot{pull.HeadSHA: snapshotWithTree('e')},
	}

	_, err := NewService(remote, &fakeGit{}).Propose(
		context.Background(), proposalRequest(local, strings.Repeat("a", 40)),
	)
	if !errors.Is(err, ErrDiverged) {
		t.Fatalf("Propose() error = %v, want ErrDiverged", err)
	}
}

func TestServiceRecoversOrphanBranchAfterPullRequestCreationFailure(t *testing.T) {
	local := snapshotWithTree('b')
	request := proposalRequest(local, strings.Repeat("a", 40))
	branch := BranchName(BranchPrefix(request.SkillName, request.SourcePath), request.BaseTreeSHA, local.TreeSHA, 1)
	head := strings.Repeat("c", 40)
	remote := &fakeRemote{
		byHead: map[string][]PullRequest{},
		skills: map[string]source.SkillSnapshot{head: local},
	}
	git := &fakeGit{refs: map[string]string{"refs/heads/" + branch: head}}

	result, err := NewService(remote, git).Propose(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Created || len(git.requests) != 0 || len(remote.createCalls) != 1 || remote.createCalls[0].Head != branch {
		t.Fatalf("result=%#v git=%d create=%#v", result, len(git.requests), remote.createCalls)
	}
}

func TestServiceUsesNewBranchAfterClosedProposal(t *testing.T) {
	local := snapshotWithTree('b')
	request := proposalRequest(local, strings.Repeat("a", 40))
	prefix := BranchPrefix(request.SkillName, request.SourcePath)
	first := BranchName(prefix, request.BaseTreeSHA, local.TreeSHA, 1)
	remote := &fakeRemote{byHead: map[string][]PullRequest{
		"owner:" + first: {{Number: 8, State: "closed", HeadRef: first}},
	}}
	git := &fakeGit{
		refs:   map[string]string{"refs/heads/" + first: strings.Repeat("c", 40)},
		result: gitcli.PushResult{CommitSHA: strings.Repeat("d", 40), TreeSHA: local.TreeSHA, Pushed: true},
	}

	_, err := NewService(remote, git).Propose(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	want := BranchName(prefix, request.BaseTreeSHA, local.TreeSHA, 2)
	if len(git.requests) != 1 || git.requests[0].HeadBranch != want {
		t.Fatalf("requests = %#v, want branch %s", git.requests, want)
	}
}

func TestServiceRefusesMultipleActiveProposalsForSameSkill(t *testing.T) {
	local := snapshotWithTree('d')
	first := openPull(t, 'a', 'b', 'c')
	second := openPull(t, 'a', 'b', 'c')
	second.Number = 8
	second.URL = "https://github.com/owner/repo/pull/8"
	second.HeadRef += "-2"

	_, err := NewService(&fakeRemote{open: []PullRequest{first, second}}, &fakeGit{}).Propose(
		context.Background(), proposalRequest(local, strings.Repeat("a", 40)),
	)

	if !errors.Is(err, ErrAmbiguous) {
		t.Fatalf("Propose() error = %v, want ErrAmbiguous", err)
	}
}

func TestFindActiveReturnsToolOwnedProposal(t *testing.T) {
	pull := openPull(t, 'a', 'b', 'c')
	remote := &fakeRemote{open: []PullRequest{pull}}

	found, err := NewService(remote, &fakeGit{}).FindActive(
		context.Background(), source.Repository{Owner: "owner", Name: "repo"},
		"main", "sample", "skills/sample",
	)

	if err != nil || found == nil || found.Number != pull.Number {
		t.Fatalf("FindActive() = %#v, %v", found, err)
	}
}

func TestFindActiveIgnoresUnrelatedPullRequests(t *testing.T) {
	pull := openPull(t, 'a', 'b', 'c')
	pull.HeadRef = "feature/unrelated"

	found, err := NewService(&fakeRemote{open: []PullRequest{pull}}, &fakeGit{}).FindActive(
		context.Background(), source.Repository{Owner: "owner", Name: "repo"},
		"main", "sample", "skills/sample",
	)

	if err != nil || found != nil {
		t.Fatalf("FindActive() = %#v, %v; want nil", found, err)
	}
}

func TestFindMergedReturnsProposalThatProducedCurrentTree(t *testing.T) {
	pull := openPull(t, 'a', 'b', 'c')
	pull.State = "closed"
	pull.Merged = true
	remote := &fakeRemote{open: []PullRequest{pull}}

	found, err := NewService(remote, &fakeGit{}).FindMerged(
		context.Background(), source.Repository{Owner: "owner", Name: "repo"},
		"main", "sample", "skills/sample", strings.Repeat("b", 40),
	)

	if err != nil || found == nil || found.Number != pull.Number {
		t.Fatalf("FindMerged() = %#v, %v", found, err)
	}
	if len(remote.listCalls) != 1 || remote.listCalls[0].State != "all" {
		t.Fatalf("list calls = %#v", remote.listCalls)
	}
}

func TestFindMergedIgnoresDifferentProposedTree(t *testing.T) {
	pull := openPull(t, 'a', 'b', 'c')
	pull.State = "closed"
	pull.Merged = true

	found, err := NewService(&fakeRemote{open: []PullRequest{pull}}, &fakeGit{}).FindMerged(
		context.Background(), source.Repository{Owner: "owner", Name: "repo"},
		"main", "sample", "skills/sample", strings.Repeat("d", 40),
	)

	if err != nil || found != nil {
		t.Fatalf("FindMerged() = %#v, %v; want nil", found, err)
	}
}

func TestMatchingMergedPullRequiresSameRepositoryAndBase(t *testing.T) {
	local := snapshotWithTree('b')
	request := proposalRequest(local, strings.Repeat("a", 40))
	pull := openPull(t, 'a', 'b', 'c')
	pull.State = "closed"
	pull.Merged = true
	pull.BaseRepository = "other/repo"

	if got := matchingMergedPull([]PullRequest{pull}, request); got != nil {
		t.Fatalf("matchingMergedPull() = %#v, want nil", got)
	}
}

func proposalRequest(local source.SkillSnapshot, baseTree string) Request {
	return Request{
		Repository:    source.Repository{Owner: "owner", Name: "repo"},
		RepositoryURL: "https://github.com/owner/repo.git", BaseBranch: "main",
		SkillName: "sample", SourcePath: "skills/sample", BaseTreeSHA: baseTree,
		Snapshot: local, Title: "chore(skill): sync sample", Message: "chore(skill): sync sample",
	}
}

func snapshotWithTree(value byte) source.SkillSnapshot {
	return source.SkillSnapshot{
		TreeSHA:    strings.Repeat(string(value), 40),
		Files:      map[string][]byte{"SKILL.md": []byte("---\nname: sample\n---\n")},
		Executable: map[string]bool{"SKILL.md": false},
	}
}

func openPull(t *testing.T, baseTree, proposedTree, head byte) PullRequest {
	t.Helper()
	metadata := Metadata{
		Version: MetadataVersion, SourcePath: "skills/sample", BaseRef: "refs/heads/main",
		BaseTreeSHA: strings.Repeat(string(baseTree), 40), ProposedTreeSHA: strings.Repeat(string(proposedTree), 40),
		HeadCommitSHA: strings.Repeat(string(head), 40),
	}
	body, err := SetMetadata("Synchronize sample.\n", metadata)
	if err != nil {
		t.Fatal(err)
	}
	return PullRequest{
		Number: 7, URL: "https://github.com/owner/repo/pull/7", State: "open", Body: body,
		HeadRef: BranchPrefix("sample", "skills/sample") + "/old", HeadSHA: metadata.HeadCommitSHA,
		HeadRepository: "owner/repo", BaseRef: "main", BaseRepository: "owner/repo",
	}
}
