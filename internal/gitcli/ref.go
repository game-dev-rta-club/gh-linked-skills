package gitcli

import (
	"context"
	"fmt"
	"strings"
)

func (c *Client) ResolveRef(ctx context.Context, repositoryURL, ref string) (string, error) {
	sha, found, err := c.FindRef(ctx, repositoryURL, ref)
	if err != nil {
		return "", err
	}
	if !found {
		return "", fmt.Errorf("resolve remote ref %s: not found", ref)
	}
	return sha, nil
}

func (c *Client) FindRef(ctx context.Context, repositoryURL, ref string) (string, bool, error) {
	if repositoryURL == "" || ref == "" || !strings.HasPrefix(ref, "refs/") {
		return "", false, fmt.Errorf("repository URL and full ref are required")
	}
	stdout, stderr, err := c.runner.Run(ctx, "ls-remote", repositoryURL, ref)
	if err != nil {
		return "", false, fmt.Errorf("find remote ref %s: %s: %w", ref, commandDetail(stderr), err)
	}
	trimmed := strings.TrimSpace(stdout)
	if trimmed == "" {
		return "", false, nil
	}
	lines := strings.Split(trimmed, "\n")
	if len(lines) != 1 {
		return "", false, fmt.Errorf("find remote ref %s: expected one result, got %d", ref, len(lines))
	}
	fields := strings.Fields(lines[0])
	if len(fields) != 2 || fields[1] != ref || fields[0] == "" {
		return "", false, fmt.Errorf("find remote ref %s: malformed ls-remote output", ref)
	}
	return fields[0], true, nil
}
