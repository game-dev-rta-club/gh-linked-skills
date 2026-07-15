package gitcli

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/game-dev-rta-club/gh-skill-linker/internal/source"
)

type ProposalRequest struct {
	RepositoryURL           string
	BaseBranch              string
	HeadBranch              string
	SkillPath               string
	ExpectedBaseTreeSHA     string
	ExpectedHeadCommitSHA   string
	ExpectedProposalTreeSHA string
	MergeBase               bool
	Snapshot                source.SkillSnapshot
	Message                 string
}

func (c *Client) ProposeSkill(ctx context.Context, request ProposalRequest) (PushResult, error) {
	cleanPath, err := c.validateProposalRequest(ctx, request)
	if err != nil {
		return PushResult{}, err
	}
	directory, err := os.MkdirTemp("", "gh-skill-linker-proposal-")
	if err != nil {
		return PushResult{}, fmt.Errorf("create proposal directory: %w", err)
	}
	defer os.RemoveAll(directory)
	checkout := filepath.Join(directory, "repository")

	updating := request.ExpectedHeadCommitSHA != ""
	if updating {
		err = c.cloneProposalHead(ctx, checkout, request)
	} else {
		err = c.cloneProposalBase(ctx, checkout, request)
	}
	if err != nil {
		return PushResult{}, err
	}
	if err := c.requireTree(ctx, checkout, proposalBaseRevision(updating, request.BaseBranch), cleanPath, request.ExpectedBaseTreeSHA); err != nil {
		return PushResult{}, fmt.Errorf("verify proposal base: %w", err)
	}

	mergeInProgress := false
	if updating && request.MergeBase {
		mergeInProgress, err = c.mergeProposalBase(ctx, checkout, cleanPath, request.BaseBranch)
		if err != nil {
			return PushResult{}, err
		}
	}
	if err := replaceProposalSnapshot(checkout, cleanPath, request.Snapshot); err != nil {
		return PushResult{}, err
	}
	if _, stderr, err := c.runner.Run(ctx, "-C", checkout, "add", "-A", "--", cleanPath); err != nil {
		return PushResult{}, fmt.Errorf("stage proposed skill: %s: %w", commandDetail(stderr), err)
	}
	if mergeInProgress {
		if err := c.requireNoUnmergedPaths(ctx, checkout); err != nil {
			return PushResult{}, err
		}
	}

	changed, err := c.proposalHasChanges(ctx, checkout, cleanPath, mergeInProgress)
	if err != nil {
		return PushResult{}, err
	}
	if !changed {
		commitSHA, treeSHA, err := c.readProposalResult(ctx, checkout, cleanPath)
		return PushResult{CommitSHA: commitSHA, TreeSHA: treeSHA}, err
	}
	commitArgs := []string{
		"-C", checkout,
		"-c", "user.name=gh-skill-linker",
		"-c", "user.email=gh-skill-linker@users.noreply.github.com",
		"commit", "-m", request.Message,
	}
	if !mergeInProgress {
		commitArgs = append(commitArgs, "--", cleanPath)
	}
	if _, stderr, err := c.runner.Run(ctx, commitArgs...); err != nil {
		return PushResult{}, fmt.Errorf("commit proposed skill: %s: %w", commandDetail(stderr), err)
	}
	if _, stderr, err := c.runner.Run(
		ctx, "-C", checkout, "push", "origin", "HEAD:refs/heads/"+request.HeadBranch,
	); err != nil {
		if remoteAdvanced(stderr) {
			return PushResult{}, fmt.Errorf("%w: proposal branch advanced after clone", ErrRemoteChanged)
		}
		return PushResult{}, fmt.Errorf("push proposal branch: %s: %w", commandDetail(stderr), err)
	}
	commitSHA, treeSHA, err := c.readProposalResult(ctx, checkout, cleanPath)
	if err != nil {
		return PushResult{}, err
	}
	if treeSHA != request.Snapshot.TreeSHA {
		return PushResult{}, fmt.Errorf("proposed skill tree mismatch: expected %s, wrote %s", request.Snapshot.TreeSHA, treeSHA)
	}
	return PushResult{CommitSHA: commitSHA, TreeSHA: treeSHA, Pushed: true}, nil
}

func (c *Client) validateProposalRequest(ctx context.Context, request ProposalRequest) (string, error) {
	if request.RepositoryURL == "" || request.BaseBranch == "" || request.HeadBranch == "" || request.Message == "" {
		return "", fmt.Errorf("repository URL, base branch, head branch, and commit message are required")
	}
	if request.BaseBranch == request.HeadBranch {
		return "", fmt.Errorf("proposal head branch must differ from its base")
	}
	for _, branch := range []string{request.BaseBranch, request.HeadBranch} {
		if strings.HasPrefix(branch, "-") {
			return "", fmt.Errorf("invalid branch %q", branch)
		}
		if _, stderr, err := c.runner.Run(ctx, "check-ref-format", "--branch", branch); err != nil {
			return "", fmt.Errorf("invalid branch %q: %s: %w", branch, commandDetail(stderr), err)
		}
	}
	if (request.ExpectedHeadCommitSHA == "") != (request.ExpectedProposalTreeSHA == "") {
		return "", fmt.Errorf("expected proposal head commit and tree must be provided together")
	}
	if request.MergeBase && request.ExpectedHeadCommitSHA == "" {
		return "", fmt.Errorf("base merge requires an existing proposal branch")
	}
	cleanPath, err := validatePublishSnapshot(request.SkillPath, request.Snapshot)
	if err != nil {
		return "", err
	}
	if request.Snapshot.TreeSHA == "" {
		return "", fmt.Errorf("proposal snapshot tree SHA is required")
	}
	return cleanPath, nil
}

func (c *Client) cloneProposalBase(ctx context.Context, checkout string, request ProposalRequest) error {
	if _, stderr, err := c.runner.Run(
		ctx, "clone", "--branch", request.BaseBranch, "--single-branch", "--no-tags", request.RepositoryURL, checkout,
	); err != nil {
		return fmt.Errorf("clone proposal base branch: %s: %w", commandDetail(stderr), err)
	}
	if _, stderr, err := c.runner.Run(ctx, "-C", checkout, "checkout", "-b", request.HeadBranch); err != nil {
		return fmt.Errorf("create proposal branch: %s: %w", commandDetail(stderr), err)
	}
	return nil
}

func (c *Client) cloneProposalHead(ctx context.Context, checkout string, request ProposalRequest) error {
	if _, stderr, err := c.runner.Run(
		ctx, "clone", "--branch", request.HeadBranch, "--single-branch", "--no-tags", request.RepositoryURL, checkout,
	); err != nil {
		return fmt.Errorf("clone proposal branch: %s: %w", commandDetail(stderr), err)
	}
	head, stderr, err := c.runner.Run(ctx, "-C", checkout, "rev-parse", "HEAD")
	if err != nil {
		return fmt.Errorf("read proposal head: %s: %w", commandDetail(stderr), err)
	}
	if strings.TrimSpace(head) != request.ExpectedHeadCommitSHA {
		return fmt.Errorf("%w: proposal head changed", ErrRemoteChanged)
	}
	if err := c.requireTree(ctx, checkout, "HEAD", request.SkillPath, request.ExpectedProposalTreeSHA); err != nil {
		return fmt.Errorf("%w: proposal skill changed: %v", ErrRemoteChanged, err)
	}
	if _, stderr, err := c.runner.Run(
		ctx, "-C", checkout, "fetch", "--no-tags", "origin",
		"refs/heads/"+request.BaseBranch+":refs/remotes/origin/"+request.BaseBranch,
	); err != nil {
		return fmt.Errorf("fetch proposal base branch: %s: %w", commandDetail(stderr), err)
	}
	return nil
}

func proposalBaseRevision(updating bool, baseBranch string) string {
	if updating {
		return "refs/remotes/origin/" + baseBranch
	}
	return "HEAD"
}

func (c *Client) requireTree(ctx context.Context, checkout, revision, skillPath, expected string) error {
	object := revision + ":" + skillPath
	objectType, stderr, err := c.runner.Run(ctx, "-C", checkout, "cat-file", "-t", object)
	if expected == "" {
		if err == nil {
			return fmt.Errorf("target already exists at %s", skillPath)
		}
		if isExitCode(err, 128) {
			return nil
		}
		return fmt.Errorf("inspect target %s: %s: %w", skillPath, commandDetail(stderr), err)
	}
	if err != nil {
		return fmt.Errorf("read tree %s: %s: %w", skillPath, commandDetail(stderr), err)
	}
	if strings.TrimSpace(objectType) != "tree" {
		return fmt.Errorf("target %s is not a tree", skillPath)
	}
	actual, stderr, err := c.runner.Run(ctx, "-C", checkout, "rev-parse", object)
	if err != nil {
		return fmt.Errorf("read tree SHA %s: %s: %w", skillPath, commandDetail(stderr), err)
	}
	if strings.TrimSpace(actual) != expected {
		return fmt.Errorf("expected %s, found %s", expected, strings.TrimSpace(actual))
	}
	return nil
}

func (c *Client) mergeProposalBase(ctx context.Context, checkout, skillPath, baseBranch string) (bool, error) {
	revision := "refs/remotes/origin/" + baseBranch
	_, stderr, err := c.runner.Run(ctx, "-C", checkout, "merge", "--no-commit", "--no-ff", revision)
	if err != nil {
		paths, pathsErr := c.unmergedPaths(ctx, checkout)
		if pathsErr != nil {
			return false, pathsErr
		}
		if len(paths) == 0 {
			return false, fmt.Errorf("merge proposal base: %s: %w", commandDetail(stderr), err)
		}
		for _, conflictPath := range paths {
			if conflictPath != skillPath && !strings.HasPrefix(conflictPath, skillPath+"/") {
				return false, fmt.Errorf("merge proposal base has conflict outside skill: %s", conflictPath)
			}
		}
	}
	if _, _, verifyErr := c.runner.Run(ctx, "-C", checkout, "rev-parse", "--verify", "MERGE_HEAD"); verifyErr != nil {
		return false, nil
	}
	return true, nil
}

func replaceProposalSnapshot(checkout, skillPath string, snapshot source.SkillSnapshot) error {
	target := filepath.Join(checkout, filepath.FromSlash(skillPath))
	if err := os.RemoveAll(target); err != nil {
		return fmt.Errorf("clear proposed skill: %w", err)
	}
	return writePublishedSnapshot(checkout, skillPath, snapshot)
}

func (c *Client) unmergedPaths(ctx context.Context, checkout string) ([]string, error) {
	stdout, stderr, err := c.runner.Run(ctx, "-C", checkout, "diff", "--name-only", "--diff-filter=U", "-z")
	if err != nil {
		return nil, fmt.Errorf("inspect proposal merge conflicts: %s: %w", commandDetail(stderr), err)
	}
	if stdout == "" {
		return nil, nil
	}
	paths := strings.Split(stdout, "\x00")
	if paths[len(paths)-1] == "" {
		paths = paths[:len(paths)-1]
	}
	return paths, nil
}

func (c *Client) requireNoUnmergedPaths(ctx context.Context, checkout string) error {
	paths, err := c.unmergedPaths(ctx, checkout)
	if err != nil {
		return err
	}
	if len(paths) > 0 {
		return fmt.Errorf("proposal merge remains unresolved: %s", strings.Join(paths, ", "))
	}
	return nil
}

func (c *Client) proposalHasChanges(ctx context.Context, checkout, skillPath string, mergeInProgress bool) (bool, error) {
	if mergeInProgress {
		return true, nil
	}
	_, stderr, err := c.runner.Run(ctx, "-C", checkout, "diff", "--cached", "--quiet", "--", skillPath)
	if err == nil {
		return false, nil
	}
	if isExitCode(err, 1) {
		return true, nil
	}
	return false, fmt.Errorf("inspect proposed skill changes: %s: %w", commandDetail(stderr), err)
}

func (c *Client) readProposalResult(ctx context.Context, checkout, skillPath string) (string, string, error) {
	commit, stderr, err := c.runner.Run(ctx, "-C", checkout, "rev-parse", "HEAD")
	if err != nil {
		return "", "", fmt.Errorf("read proposal commit: %s: %w", commandDetail(stderr), err)
	}
	tree, stderr, err := c.runner.Run(ctx, "-C", checkout, "rev-parse", "HEAD:"+skillPath)
	if err != nil {
		return "", "", fmt.Errorf("read proposed skill tree: %s: %w", commandDetail(stderr), err)
	}
	return strings.TrimSpace(commit), strings.TrimSpace(tree), nil
}
