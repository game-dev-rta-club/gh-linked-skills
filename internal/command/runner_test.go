package command

import (
	"context"
	"strings"
	"testing"
)

func TestRunnerDisablesGitTraceAndRedactsSecrets(t *testing.T) {
	const secret = "token-value"
	runner := NewWithEnv("/bin/sh", map[string]string{"SECRET": secret}, secret)

	_, stderr, err := runner.Run(context.Background(), "-c", `printf '%s %s' "$SECRET" "$GIT_TRACE_CURL" >&2; exit 1`)

	if err == nil {
		t.Fatal("Run() error = nil, want shell failure")
	}
	if strings.Contains(stderr, secret) {
		t.Fatalf("stderr leaked secret: %q", stderr)
	}
	if !strings.Contains(stderr, "[REDACTED] 0") {
		t.Fatalf("stderr = %q, want redaction and disabled trace", stderr)
	}
}
