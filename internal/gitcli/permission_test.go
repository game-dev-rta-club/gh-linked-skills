package gitcli

import (
	"context"
	"errors"
	"testing"
)

type sequenceResponse struct {
	stderr string
	err    error
}

type sequenceRunner struct {
	responses []sequenceResponse
	calls     int
}

func (r *sequenceRunner) Run(context.Context, ...string) (string, string, error) {
	response := r.responses[r.calls]
	r.calls++
	return "", response.stderr, response.err
}

func TestCanPushReturnsFalseForKnownPermissionDenial(t *testing.T) {
	runner := &sequenceRunner{responses: []sequenceResponse{
		{},
		{stderr: "remote: Write access to repository not granted.", err: errors.New("exit status 128")},
	}}

	canPush, err := New(runner).CanPush(context.Background(), "https://github.com/owner/repo.git", "main")

	if err != nil {
		t.Fatalf("CanPush() error = %v", err)
	}
	if canPush {
		t.Fatal("CanPush() = true, want false")
	}
}

func TestCanPushReportsUnknownProbeFailure(t *testing.T) {
	runner := &sequenceRunner{responses: []sequenceResponse{
		{},
		{stderr: "connection reset", err: errors.New("exit status 128")},
	}}

	_, err := New(runner).CanPush(context.Background(), "https://github.com/owner/repo.git", "main")

	if err == nil {
		t.Fatal("CanPush() error = nil, want probe failure")
	}
}
