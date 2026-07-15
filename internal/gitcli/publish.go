package gitcli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/game-dev-rta-club/gh-skill-linker/internal/source"
)

var (
	ErrTargetExists  = errors.New("source path already exists with different content")
	ErrBranchMissing = errors.New("source branch does not exist")
)

func (c *Client) PublishSkill(
	ctx context.Context,
	repositoryURL string,
	branch string,
	skillPath string,
	snapshot source.SkillSnapshot,
	message string,
) (PushResult, error) {
	if repositoryURL == "" || snapshot.TreeSHA == "" || message == "" {
		return PushResult{}, fmt.Errorf("repository URL, skill tree SHA, and commit message are required")
	}
	if branch == "" || strings.HasPrefix(branch, "-") {
		return PushResult{}, fmt.Errorf("invalid branch %q", branch)
	}
	if _, stderr, err := c.runner.Run(ctx, "check-ref-format", "--branch", branch); err != nil {
		return PushResult{}, fmt.Errorf("invalid branch %q: %s: %w", branch, commandDetail(stderr), err)
	}
	cleanSkillPath, err := validatePublishSnapshot(skillPath, snapshot)
	if err != nil {
		return PushResult{}, err
	}

	refs, stderr, err := c.runner.Run(ctx, "ls-remote", repositoryURL)
	if err != nil {
		return PushResult{}, fmt.Errorf("inspect source repository: %s: %w", commandDetail(stderr), err)
	}
	branchExists := remoteRefExists(refs, "refs/heads/"+branch)
	if !branchExists && strings.TrimSpace(refs) != "" {
		return PushResult{}, fmt.Errorf("%w: %s", ErrBranchMissing, branch)
	}

	directory, err := os.MkdirTemp("", "gh-skill-linker-publish-")
	if err != nil {
		return PushResult{}, fmt.Errorf("create publish directory: %w", err)
	}
	defer os.RemoveAll(directory)
	checkout := filepath.Join(directory, "repository")
	if branchExists {
		if _, stderr, err := c.runner.Run(
			ctx, "clone", "--branch", branch, "--single-branch", "--no-tags", "--depth", "1", repositoryURL, checkout,
		); err != nil {
			return PushResult{}, fmt.Errorf("clone source branch: %s: %w", commandDetail(stderr), err)
		}
		if result, found, err := c.existingPublishTarget(ctx, checkout, cleanSkillPath, snapshot.TreeSHA); err != nil {
			return PushResult{}, err
		} else if found {
			return result, nil
		}
	} else {
		if _, stderr, err := c.runner.Run(ctx, "clone", "--no-tags", repositoryURL, checkout); err != nil {
			return PushResult{}, fmt.Errorf("clone empty source repository: %s: %w", commandDetail(stderr), err)
		}
		if _, stderr, err := c.runner.Run(ctx, "-C", checkout, "rev-parse", "--verify", "HEAD"); err == nil {
			return PushResult{}, fmt.Errorf("%w: repository gained a commit during publish", ErrRemoteChanged)
		} else if !isExitCode(err, 128) {
			return PushResult{}, fmt.Errorf("inspect empty source repository: %s: %w", commandDetail(stderr), err)
		}
		currentRefs, stderr, err := c.runner.Run(ctx, "ls-remote", repositoryURL)
		if err != nil {
			return PushResult{}, fmt.Errorf("recheck empty source repository: %s: %w", commandDetail(stderr), err)
		}
		if strings.TrimSpace(currentRefs) != "" {
			return PushResult{}, fmt.Errorf("%w: repository gained a ref during publish", ErrRemoteChanged)
		}
		if _, stderr, err := c.runner.Run(ctx, "-C", checkout, "checkout", "--orphan", branch); err != nil {
			return PushResult{}, fmt.Errorf("initialize source branch %s: %s: %w", branch, commandDetail(stderr), err)
		}
	}

	if err := writePublishedSnapshot(checkout, cleanSkillPath, snapshot); err != nil {
		return PushResult{}, err
	}
	if _, stderr, err := c.runner.Run(ctx, "-C", checkout, "add", "-A", "--", cleanSkillPath); err != nil {
		return PushResult{}, fmt.Errorf("stage published skill: %s: %w", commandDetail(stderr), err)
	}
	if _, stderr, err := c.runner.Run(
		ctx,
		"-C", checkout,
		"-c", "user.name=gh-skill-linker",
		"-c", "user.email=gh-skill-linker@users.noreply.github.com",
		"commit", "-m", message, "--", cleanSkillPath,
	); err != nil {
		return PushResult{}, fmt.Errorf("commit published skill: %s: %w", commandDetail(stderr), err)
	}
	localTree, stderr, err := c.runner.Run(ctx, "-C", checkout, "rev-parse", "HEAD:"+cleanSkillPath)
	if err != nil {
		return PushResult{}, fmt.Errorf("read published skill tree: %s: %w", commandDetail(stderr), err)
	}
	localTree = strings.TrimSpace(localTree)
	if localTree != snapshot.TreeSHA {
		return PushResult{}, fmt.Errorf("published skill tree mismatch: expected %s, wrote %s", snapshot.TreeSHA, localTree)
	}
	if _, stderr, err := c.runner.Run(ctx, "-C", checkout, "push", "origin", "HEAD:refs/heads/"+branch); err != nil {
		if remoteAdvanced(stderr) {
			return PushResult{}, fmt.Errorf("%w: source branch advanced during publish", ErrRemoteChanged)
		}
		return PushResult{}, fmt.Errorf("push published skill: %s: %w", commandDetail(stderr), err)
	}
	commitSHA, stderr, err := c.runner.Run(ctx, "-C", checkout, "rev-parse", "HEAD")
	if err != nil {
		return PushResult{}, fmt.Errorf("read published commit: %s: %w", commandDetail(stderr), err)
	}
	return PushResult{CommitSHA: strings.TrimSpace(commitSHA), TreeSHA: localTree, Pushed: true}, nil
}

func validatePublishSnapshot(skillPath string, snapshot source.SkillSnapshot) (string, error) {
	cleanSkillPath, err := validateRepositoryPath(skillPath)
	if err != nil {
		return "", err
	}
	if _, ok := snapshot.Files["SKILL.md"]; !ok {
		return "", fmt.Errorf("source snapshot has no SKILL.md")
	}
	for relative := range snapshot.Files {
		if _, err := validateRepositoryPath(relative); err != nil {
			return "", fmt.Errorf("invalid source file: %w", err)
		}
	}
	return cleanSkillPath, nil
}

func remoteRefExists(output, ref string) bool {
	for _, line := range strings.Split(output, "\n") {
		fields := strings.Fields(line)
		if len(fields) == 2 && fields[1] == ref {
			return true
		}
	}
	return false
}

func (c *Client) existingPublishTarget(
	ctx context.Context,
	checkout string,
	skillPath string,
	expectedTree string,
) (PushResult, bool, error) {
	objectType, typeStderr, typeErr := c.runner.Run(ctx, "-C", checkout, "cat-file", "-t", "HEAD:"+skillPath)
	if typeErr == nil {
		if strings.TrimSpace(objectType) != "tree" {
			return PushResult{}, false, fmt.Errorf("%w: %s is not a directory", ErrTargetExists, skillPath)
		}
		treeSHA, stderr, err := c.runner.Run(ctx, "-C", checkout, "rev-parse", "HEAD:"+skillPath)
		if err != nil {
			return PushResult{}, false, fmt.Errorf("read existing source tree: %s: %w", commandDetail(stderr), err)
		}
		treeSHA = strings.TrimSpace(treeSHA)
		if treeSHA != expectedTree {
			return PushResult{}, false, fmt.Errorf("%w: %s", ErrTargetExists, skillPath)
		}
		commitSHA, stderr, err := c.runner.Run(ctx, "-C", checkout, "rev-parse", "HEAD")
		if err != nil {
			return PushResult{}, false, fmt.Errorf("read existing source commit: %s: %w", commandDetail(stderr), err)
		}
		return PushResult{CommitSHA: strings.TrimSpace(commitSHA), TreeSHA: treeSHA}, true, nil
	}
	if !isExitCode(typeErr, 128) {
		return PushResult{}, false, fmt.Errorf("inspect existing source path: %s: %w", commandDetail(typeStderr), typeErr)
	}

	components := strings.Split(skillPath, "/")
	for index := 1; index < len(components); index++ {
		ancestor := strings.Join(components[:index], "/")
		objectType, stderr, err := c.runner.Run(ctx, "-C", checkout, "cat-file", "-t", "HEAD:"+ancestor)
		if err == nil && strings.TrimSpace(objectType) != "tree" {
			return PushResult{}, false, fmt.Errorf("%w: ancestor %s is not a directory", ErrTargetExists, ancestor)
		}
		if err != nil && !isExitCode(err, 128) {
			return PushResult{}, false, fmt.Errorf("inspect source ancestor %s: %s: %w", ancestor, commandDetail(stderr), err)
		}
	}
	return PushResult{}, false, nil
}

func writePublishedSnapshot(checkout, skillPath string, snapshot source.SkillSnapshot) error {
	target := filepath.Join(checkout, filepath.FromSlash(skillPath))
	if err := os.MkdirAll(target, 0o755); err != nil {
		return fmt.Errorf("create published skill directory: %w", err)
	}
	for relative, content := range snapshot.Files {
		cleanRelative := path.Clean(relative)
		destination := filepath.Join(target, filepath.FromSlash(cleanRelative))
		if err := os.MkdirAll(filepath.Dir(destination), 0o755); err != nil {
			return fmt.Errorf("create published directory for %s: %w", relative, err)
		}
		mode := os.FileMode(0o644)
		if snapshot.Executable[relative] {
			mode = 0o755
		}
		if err := os.WriteFile(destination, content, mode); err != nil {
			return fmt.Errorf("write published file %s: %w", relative, err)
		}
	}
	return nil
}
