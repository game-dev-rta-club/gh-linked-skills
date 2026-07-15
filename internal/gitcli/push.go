package gitcli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strings"

	"github.com/game-dev-rta-club/gh-linked-skills/internal/source"
)

var ErrRemoteChanged = errors.New("remote skill changed")

type PushResult struct {
	CommitSHA string
	TreeSHA   string
	Pushed    bool
}

func (c *Client) PushSkill(
	ctx context.Context,
	repositoryURL string,
	branch string,
	skillPath string,
	expectedTreeSHA string,
	snapshot source.SkillSnapshot,
	message string,
) (PushResult, error) {
	if repositoryURL == "" || expectedTreeSHA == "" || message == "" {
		return PushResult{}, fmt.Errorf("repository URL, expected tree SHA, and commit message are required")
	}
	if branch == "" || strings.HasPrefix(branch, "-") {
		return PushResult{}, fmt.Errorf("invalid branch %q", branch)
	}
	if _, stderr, err := c.runner.Run(ctx, "check-ref-format", "--branch", branch); err != nil {
		return PushResult{}, fmt.Errorf("invalid branch %q: %s: %w", branch, commandDetail(stderr), err)
	}
	cleanSkillPath, err := validateRepositoryPath(skillPath)
	if err != nil {
		return PushResult{}, err
	}
	if _, ok := snapshot.Files["SKILL.md"]; !ok {
		return PushResult{}, fmt.Errorf("source snapshot has no SKILL.md")
	}
	for relative := range snapshot.Files {
		if _, err := validateRepositoryPath(relative); err != nil {
			return PushResult{}, fmt.Errorf("invalid source file: %w", err)
		}
	}

	directory, err := os.MkdirTemp("", "gh-linked-skills-push-")
	if err != nil {
		return PushResult{}, fmt.Errorf("create push directory: %w", err)
	}
	defer os.RemoveAll(directory)
	checkout := filepath.Join(directory, "repository")
	if _, stderr, err := c.runner.Run(
		ctx,
		"clone", "--branch", branch, "--single-branch", "--no-tags", "--depth", "1",
		repositoryURL, checkout,
	); err != nil {
		return PushResult{}, fmt.Errorf("clone source branch: %s: %w", commandDetail(stderr), err)
	}

	currentTree, stderr, err := c.runner.Run(ctx, "-C", checkout, "rev-parse", "HEAD:"+cleanSkillPath)
	if err != nil {
		return PushResult{}, fmt.Errorf("read cloned skill tree: %s: %w", commandDetail(stderr), err)
	}
	currentTree = strings.TrimSpace(currentTree)
	if currentTree != expectedTreeSHA {
		return PushResult{}, fmt.Errorf("%w: expected %s, cloned %s", ErrRemoteChanged, expectedTreeSHA, currentTree)
	}

	target := filepath.Join(checkout, filepath.FromSlash(cleanSkillPath))
	info, err := os.Lstat(target)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return PushResult{}, fmt.Errorf("cloned skill path is not a regular directory: %s", cleanSkillPath)
	}
	if err := os.RemoveAll(target); err != nil {
		return PushResult{}, fmt.Errorf("clear cloned skill: %w", err)
	}
	if err := os.MkdirAll(target, 0o755); err != nil {
		return PushResult{}, fmt.Errorf("create cloned skill: %w", err)
	}
	for relative, content := range snapshot.Files {
		destination := filepath.Join(target, filepath.FromSlash(relative))
		if err := os.MkdirAll(filepath.Dir(destination), 0o755); err != nil {
			return PushResult{}, fmt.Errorf("create source directory: %w", err)
		}
		mode := os.FileMode(0o644)
		if snapshot.Executable[relative] {
			mode = 0o755
		}
		if err := os.WriteFile(destination, content, mode); err != nil {
			return PushResult{}, fmt.Errorf("write source file %s: %w", relative, err)
		}
	}
	if _, stderr, err := c.runner.Run(ctx, "-C", checkout, "add", "-A", "--", cleanSkillPath); err != nil {
		return PushResult{}, fmt.Errorf("stage source skill: %s: %w", commandDetail(stderr), err)
	}
	_, stderr, diffErr := c.runner.Run(ctx, "-C", checkout, "diff", "--cached", "--quiet", "--", cleanSkillPath)
	if diffErr == nil {
		commitSHA, stderr, err := c.runner.Run(ctx, "-C", checkout, "rev-parse", "HEAD")
		if err != nil {
			return PushResult{}, fmt.Errorf("read source commit: %s: %w", commandDetail(stderr), err)
		}
		return PushResult{CommitSHA: strings.TrimSpace(commitSHA), TreeSHA: currentTree}, nil
	}
	if !isExitCode(diffErr, 1) {
		return PushResult{}, fmt.Errorf("inspect staged source skill: %s: %w", commandDetail(stderr), diffErr)
	}
	if _, stderr, err := c.runner.Run(
		ctx,
		"-C", checkout,
		"-c", "user.name=gh-linked-skills",
		"-c", "user.email=gh-linked-skills@users.noreply.github.com",
		"commit", "-m", message, "--", cleanSkillPath,
	); err != nil {
		return PushResult{}, fmt.Errorf("commit source skill: %s: %w", commandDetail(stderr), err)
	}
	if _, stderr, err := c.runner.Run(ctx, "-C", checkout, "push", "origin", "HEAD:refs/heads/"+branch); err != nil {
		if remoteAdvanced(stderr) {
			return PushResult{}, fmt.Errorf("%w: source branch advanced after clone", ErrRemoteChanged)
		}
		return PushResult{}, fmt.Errorf("push source branch: %s: %w", commandDetail(stderr), err)
	}
	newTree, stderr, err := c.runner.Run(ctx, "-C", checkout, "rev-parse", "HEAD:"+cleanSkillPath)
	if err != nil {
		return PushResult{}, fmt.Errorf("read pushed skill tree: %s: %w", commandDetail(stderr), err)
	}
	newCommit, stderr, err := c.runner.Run(ctx, "-C", checkout, "rev-parse", "HEAD")
	if err != nil {
		return PushResult{}, fmt.Errorf("read pushed commit: %s: %w", commandDetail(stderr), err)
	}
	return PushResult{CommitSHA: strings.TrimSpace(newCommit), TreeSHA: strings.TrimSpace(newTree), Pushed: true}, nil
}

func remoteAdvanced(stderr string) bool {
	lower := strings.ToLower(stderr)
	return strings.Contains(lower, "non-fast-forward") ||
		strings.Contains(lower, "fetch first") ||
		strings.Contains(lower, "stale info")
}

func validateRepositoryPath(value string) (string, error) {
	if value == "" || strings.Contains(value, "\\") || strings.HasPrefix(value, "/") {
		return "", fmt.Errorf("unsafe repository path %q", value)
	}
	clean := path.Clean(value)
	if clean == "." || clean == ".." || strings.HasPrefix(clean, "../") || clean != value {
		return "", fmt.Errorf("unsafe repository path %q", value)
	}
	return clean, nil
}

func isExitCode(err error, code int) bool {
	var exitError *exec.ExitError
	return errors.As(err, &exitError) && exitError.ExitCode() == code
}
