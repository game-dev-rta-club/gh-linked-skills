package gitcli

import (
	"context"
	"fmt"
	"strings"
)

func (c *Client) ResolveRef(ctx context.Context, repositoryURL, ref string) (string, error) {
	if repositoryURL == "" || ref == "" || !strings.HasPrefix(ref, "refs/") {
		return "", fmt.Errorf("repository URL and full ref are required")
	}
	stdout, stderr, err := c.runner.Run(ctx, "ls-remote", "--exit-code", repositoryURL, ref)
	if err != nil {
		return "", fmt.Errorf("resolve remote ref %s: %s: %w", ref, commandDetail(stderr), err)
	}
	lines := strings.Split(strings.TrimSpace(stdout), "\n")
	if len(lines) != 1 {
		return "", fmt.Errorf("resolve remote ref %s: expected one result, got %d", ref, len(lines))
	}
	fields := strings.Fields(lines[0])
	if len(fields) != 2 || fields[1] != ref || fields[0] == "" {
		return "", fmt.Errorf("resolve remote ref %s: malformed ls-remote output", ref)
	}
	return fields[0], nil
}
