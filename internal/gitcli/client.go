package gitcli

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
)

type Runner interface {
	Run(ctx context.Context, args ...string) (stdout string, stderr string, err error)
}

type Client struct {
	runner Runner
}

func New(runner Runner) *Client {
	return &Client{runner: runner}
}

func (c *Client) Root(ctx context.Context) (string, error) {
	stdout, stderr, err := c.runner.Run(ctx, "rev-parse", "--show-toplevel")
	if err != nil {
		detail := strings.TrimSpace(stderr)
		if detail == "" {
			detail = "command failed"
		}
		return "", fmt.Errorf("find project Git root: %s: %w", detail, err)
	}
	root := strings.TrimSpace(stdout)
	if root == "" {
		return "", fmt.Errorf("find project Git root: git returned an empty path")
	}
	return root, nil
}

func (c *Client) TrackedFiles(ctx context.Context, projectRoot, relativePath string) ([]string, error) {
	return c.listFiles(ctx, projectRoot, relativePath, "--cached")
}

func (c *Client) PushFiles(ctx context.Context, projectRoot, relativePath string) ([]string, error) {
	return c.listFiles(ctx, projectRoot, relativePath, "--cached", "--others", "--exclude-standard")
}

func (c *Client) listFiles(ctx context.Context, projectRoot, relativePath string, flags ...string) ([]string, error) {
	clean := filepath.Clean(relativePath)
	if clean == "." || clean == ".." || filepath.IsAbs(clean) || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return nil, fmt.Errorf("unsafe project-relative path %q", relativePath)
	}
	args := []string{"-C", projectRoot, "ls-files", "-z"}
	args = append(args, flags...)
	args = append(args, "--", filepath.ToSlash(clean))
	stdout, stderr, err := c.runner.Run(ctx, args...)
	if err != nil {
		detail := strings.TrimSpace(stderr)
		if detail == "" {
			detail = "command failed"
		}
		return nil, fmt.Errorf("list tracked skill files: %s: %w", detail, err)
	}
	if stdout == "" {
		return []string{}, nil
	}
	parts := strings.Split(stdout, "\x00")
	if parts[len(parts)-1] == "" {
		parts = parts[:len(parts)-1]
	}
	return parts, nil
}
