package gitcli

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/game-dev-rta-club/gh-skill-linker/internal/command"
)

func TestProposeSkillCreatesBranchWithoutChangingBase(t *testing.T) {
	bare, baseTree := createBareSkillRepository(t)
	baseCommit := strings.TrimSpace(gitOutput(t, "--git-dir", bare, "rev-parse", "main"))
	snapshot := publishSnapshot(t, "Proposal A\n")

	result, err := New(command.New("git")).ProposeSkill(context.Background(), ProposalRequest{
		RepositoryURL: bare, BaseBranch: "main", HeadBranch: "skill-linker/sample/one",
		SkillPath: "skills/sample", ExpectedBaseTreeSHA: baseTree,
		Snapshot: snapshot, Message: "chore(skill): propose sample",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !result.Pushed || result.TreeSHA != snapshot.TreeSHA || result.CommitSHA == "" {
		t.Fatalf("result = %#v", result)
	}
	if got := strings.TrimSpace(gitOutput(t, "--git-dir", bare, "rev-parse", "main")); got != baseCommit {
		t.Fatalf("main changed: %s -> %s", baseCommit, got)
	}
	if got := gitOutput(t, "--git-dir", bare, "show", "skill-linker/sample/one:skills/sample/SKILL.md"); !strings.Contains(got, "Proposal A") {
		t.Fatalf("proposal body = %q", got)
	}
}

func TestProposeSkillUpdatesSameBranchWithNormalPush(t *testing.T) {
	bare, baseTree := createBareSkillRepository(t)
	client := New(command.New("git"))
	first := publishSnapshot(t, "Proposal A\n")
	created, err := client.ProposeSkill(context.Background(), ProposalRequest{
		RepositoryURL: bare, BaseBranch: "main", HeadBranch: "skill-linker/sample/one",
		SkillPath: "skills/sample", ExpectedBaseTreeSHA: baseTree,
		Snapshot: first, Message: "chore(skill): propose sample",
	})
	if err != nil {
		t.Fatal(err)
	}
	second := publishSnapshot(t, "Proposal A\nProposal B\n")

	updated, err := client.ProposeSkill(context.Background(), ProposalRequest{
		RepositoryURL: bare, BaseBranch: "main", HeadBranch: "skill-linker/sample/one",
		SkillPath: "skills/sample", ExpectedBaseTreeSHA: baseTree,
		ExpectedHeadCommitSHA: created.CommitSHA, ExpectedProposalTreeSHA: created.TreeSHA,
		Snapshot: second, Message: "chore(skill): update sample proposal",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !updated.Pushed || updated.TreeSHA != second.TreeSHA || updated.CommitSHA == created.CommitSHA {
		t.Fatalf("updated = %#v", updated)
	}
	parents := strings.Fields(strings.TrimSpace(gitOutput(t, "--git-dir", bare, "rev-list", "--parents", "-n", "1", updated.CommitSHA)))
	if len(parents) != 2 {
		t.Fatalf("parents = %q, want ordinary one-parent commit", parents)
	}
}

func TestProposeSkillMergesAdvancedBaseUsingResolvedSnapshot(t *testing.T) {
	bare, baseTree := createBareSkillRepository(t)
	client := New(command.New("git"))
	proposalA := publishSnapshot(t, "Proposal A\n")
	created, err := client.ProposeSkill(context.Background(), ProposalRequest{
		RepositoryURL: bare, BaseBranch: "main", HeadBranch: "skill-linker/sample/one",
		SkillPath: "skills/sample", ExpectedBaseTreeSHA: baseTree,
		Snapshot: proposalA, Message: "chore(skill): propose sample",
	})
	if err != nil {
		t.Fatal(err)
	}
	mainCommit, mainTree := advanceRemoteSkill(t, bare, "main", "Remote main\n", "main-only.txt")
	resolved := publishSnapshot(t, "Remote main\nProposal A\nProposal B\n")

	updated, err := client.ProposeSkill(context.Background(), ProposalRequest{
		RepositoryURL: bare, BaseBranch: "main", HeadBranch: "skill-linker/sample/one",
		SkillPath: "skills/sample", ExpectedBaseTreeSHA: mainTree,
		ExpectedHeadCommitSHA: created.CommitSHA, ExpectedProposalTreeSHA: created.TreeSHA,
		MergeBase: true, Snapshot: resolved, Message: "chore(skill): update resolved sample proposal",
	})
	if err != nil {
		t.Fatal(err)
	}
	parents := strings.Fields(strings.TrimSpace(gitOutput(t, "--git-dir", bare, "rev-list", "--parents", "-n", "1", updated.CommitSHA)))
	if len(parents) != 3 || parents[1] != created.CommitSHA || parents[2] != mainCommit {
		t.Fatalf("parents = %q, want head and current main", parents)
	}
	if got := strings.TrimSpace(gitOutput(t, "--git-dir", bare, "merge-base", updated.CommitSHA, mainCommit)); got != mainCommit {
		t.Fatalf("merge base = %s, want current main %s", got, mainCommit)
	}
	if got := gitOutput(t, "--git-dir", bare, "show", updated.CommitSHA+":main-only.txt"); got != "main update\n" {
		t.Fatalf("main-only file = %q", got)
	}
	if got := gitOutput(t, "--git-dir", bare, "show", updated.CommitSHA+":skills/sample/SKILL.md"); !strings.Contains(got, "Remote main\nProposal A\nProposal B") {
		t.Fatalf("resolved skill = %q", got)
	}
}

func TestProposeSkillRefusesExternallyAdvancedHead(t *testing.T) {
	bare, baseTree := createBareSkillRepository(t)
	client := New(command.New("git"))
	created, err := client.ProposeSkill(context.Background(), ProposalRequest{
		RepositoryURL: bare, BaseBranch: "main", HeadBranch: "skill-linker/sample/one",
		SkillPath: "skills/sample", ExpectedBaseTreeSHA: baseTree,
		Snapshot: publishSnapshot(t, "Proposal A\n"), Message: "chore(skill): propose sample",
	})
	if err != nil {
		t.Fatal(err)
	}
	advanceRemoteSkill(t, bare, "skill-linker/sample/one", "External edit\n", "")

	_, err = client.ProposeSkill(context.Background(), ProposalRequest{
		RepositoryURL: bare, BaseBranch: "main", HeadBranch: "skill-linker/sample/one",
		SkillPath: "skills/sample", ExpectedBaseTreeSHA: baseTree,
		ExpectedHeadCommitSHA: created.CommitSHA, ExpectedProposalTreeSHA: created.TreeSHA,
		Snapshot: publishSnapshot(t, "Proposal B\n"), Message: "chore(skill): update sample proposal",
	})
	if !errors.Is(err, ErrRemoteChanged) {
		t.Fatalf("ProposeSkill() error = %v, want ErrRemoteChanged", err)
	}
}

func TestProposeSkillPublishesMissingPathFromExistingBase(t *testing.T) {
	bare := createBareRepository(t)
	client := New(command.New("git"))
	snapshot := publishSnapshot(t, "Published\n")

	result, err := client.ProposeSkill(context.Background(), ProposalRequest{
		RepositoryURL: bare, BaseBranch: "main", HeadBranch: "skill-linker/sample/one",
		SkillPath: "skills/sample", Snapshot: snapshot, Message: "feat(skill): publish sample",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !result.Pushed || result.TreeSHA != snapshot.TreeSHA {
		t.Fatalf("result = %#v", result)
	}
	if got := gitOutput(t, "--git-dir", bare, "show", "skill-linker/sample/one:README.md"); got != "fixture\n" {
		t.Fatalf("README = %q", got)
	}
}

func advanceRemoteSkill(t *testing.T, bare, branch, body, extraFile string) (string, string) {
	t.Helper()
	work := filepath.Join(t.TempDir(), "advance")
	runGit(t, "clone", "--branch", branch, "--single-branch", bare, work)
	runGit(t, "-C", work, "config", "user.name", "External")
	runGit(t, "-C", work, "config", "user.email", "external@example.com")
	document := "---\nname: sample\ndescription: Sample skill.\n---\n" + body
	if err := os.WriteFile(filepath.Join(work, "skills", "sample", "SKILL.md"), []byte(document), 0o644); err != nil {
		t.Fatal(err)
	}
	if extraFile != "" {
		if err := os.WriteFile(filepath.Join(work, extraFile), []byte("main update\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	runGit(t, "-C", work, "add", ".")
	runGit(t, "-C", work, "commit", "-m", "external update")
	runGit(t, "-C", work, "push", "origin", "HEAD:refs/heads/"+branch)
	commit := strings.TrimSpace(gitOutput(t, "-C", work, "rev-parse", "HEAD"))
	tree := strings.TrimSpace(gitOutput(t, "-C", work, "rev-parse", "HEAD:skills/sample"))
	return commit, tree
}
