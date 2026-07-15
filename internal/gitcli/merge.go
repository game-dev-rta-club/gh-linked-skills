package gitcli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

func (c *Client) MergeFile(
	ctx context.Context,
	local []byte,
	base []byte,
	remote []byte,
	baseSHA string,
	remoteSHA string,
) ([]byte, bool, error) {
	directory, err := os.MkdirTemp("", "gh-linked-skills-merge-")
	if err != nil {
		return nil, false, fmt.Errorf("create merge directory: %w", err)
	}
	defer os.RemoveAll(directory)
	localPath := filepath.Join(directory, "local")
	basePath := filepath.Join(directory, "base")
	remotePath := filepath.Join(directory, "remote")
	for path, content := range map[string][]byte{localPath: local, basePath: base, remotePath: remote} {
		if err := os.WriteFile(path, content, 0o600); err != nil {
			return nil, false, fmt.Errorf("write merge input: %w", err)
		}
	}

	_, stderr, runErr := c.runner.Run(
		ctx,
		"merge-file", "--diff3",
		"-L", "gh-linked-skills:local",
		"-L", "gh-linked-skills:base:"+baseSHA,
		"-L", "gh-linked-skills:remote:"+remoteSHA,
		localPath, basePath, remotePath,
	)
	conflict := false
	if runErr != nil {
		var exitError *exec.ExitError
		if errors.As(runErr, &exitError) && exitError.ExitCode() >= 1 && exitError.ExitCode() <= 127 {
			conflict = true
		} else {
			return nil, false, fmt.Errorf("git merge-file: %s: %w", commandDetail(stderr), runErr)
		}
	}
	merged, err := os.ReadFile(localPath)
	if err != nil {
		return nil, false, fmt.Errorf("read merge result: %w", err)
	}
	return merged, conflict, nil
}

func commandDetail(stderr string) string {
	if stderr == "" {
		return "command failed"
	}
	return stderr
}
