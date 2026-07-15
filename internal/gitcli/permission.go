package gitcli

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func (c *Client) CanPush(ctx context.Context, repositoryURL, branch string) (bool, error) {
	if repositoryURL == "" || branch == "" || strings.HasPrefix(branch, "-") {
		return false, fmt.Errorf("repository URL and valid branch are required")
	}
	directory, err := os.MkdirTemp("", "gh-skill-linker-permission-")
	if err != nil {
		return false, fmt.Errorf("create permission probe directory: %w", err)
	}
	defer os.RemoveAll(directory)
	checkout := filepath.Join(directory, "repository")
	if _, stderr, err := c.runner.Run(
		ctx,
		"clone", "--branch", branch, "--single-branch", "--no-tags", "--depth", "1",
		repositoryURL, checkout,
	); err != nil {
		return false, fmt.Errorf("clone for push permission probe: %s: %w", commandDetail(stderr), err)
	}
	if _, stderr, err := c.runner.Run(
		ctx,
		"-C", checkout, "push", "--dry-run", "origin", "HEAD:refs/heads/"+branch,
	); err != nil {
		if permissionDenied(stderr) {
			return false, nil
		}
		return false, fmt.Errorf("probe push permission: %s: %w", commandDetail(stderr), err)
	}
	return true, nil
}

func permissionDenied(stderr string) bool {
	lower := strings.ToLower(stderr)
	for _, marker := range []string{
		"write access to repository not granted",
		"permission to ",
		"permission denied",
		"requested url returned error: 403",
		"access denied",
	} {
		if strings.Contains(lower, marker) {
			return true
		}
	}
	return false
}
