package compat

import (
	"context"
	"errors"
	"strings"
	"testing"
)

type scriptedResponse struct {
	stdout string
	stderr string
	err    error
}

type recordingRunner struct {
	responses []scriptedResponse
	calls     [][]string
}

func (r *recordingRunner) Run(_ context.Context, args ...string) (string, string, error) {
	r.calls = append(r.calls, append([]string(nil), args...))
	response := r.responses[len(r.calls)-1]
	return response.stdout, response.stderr, response.err
}

func TestStatusPreflightAcceptsAnyAvailableGHVersion(t *testing.T) {
	runner := &recordingRunner{responses: []scriptedResponse{{stdout: "gh version 2.95.1 (test)\n"}}}

	err := NewChecker(runner).CheckStatus(context.Background())

	if err != nil {
		t.Fatalf("CheckStatus() error = %v, want nil", err)
	}
	if len(runner.calls) != 1 || len(runner.calls[0]) != 1 || runner.calls[0][0] != "--version" {
		t.Fatalf("runner calls = %#v, want only gh --version", runner.calls)
	}
}

func TestStatusPreflightDoesNotParseVersionOutput(t *testing.T) {
	runner := &recordingRunner{responses: []scriptedResponse{{stdout: "available\n"}}}

	err := NewChecker(runner).CheckStatus(context.Background())

	if err != nil {
		t.Fatalf("CheckStatus() error = %v, want nil", err)
	}
	if len(runner.calls) != 1 || len(runner.calls[0]) != 1 || runner.calls[0][0] != "--version" {
		t.Fatalf("runner calls = %#v, want only gh --version", runner.calls)
	}
}

func TestInstallPreflightAlsoChecksAvailability(t *testing.T) {
	runner := &recordingRunner{responses: []scriptedResponse{{stdout: "available\n"}}}

	if err := NewChecker(runner).CheckInstall(context.Background()); err != nil {
		t.Fatalf("CheckInstall() error = %v, want nil", err)
	}
	if len(runner.calls) != 1 {
		t.Fatalf("runner calls = %d, want 1", len(runner.calls))
	}
}

func TestPreflightReportsVersionCommandFailure(t *testing.T) {
	runner := &recordingRunner{responses: []scriptedResponse{{stderr: "gh failed", err: errors.New("exit status 1")}}}

	err := NewChecker(runner).CheckStatus(context.Background())

	if err == nil || !strings.Contains(err.Error(), "gh failed") {
		t.Fatalf("CheckStatus() error = %v, want command detail", err)
	}
}
