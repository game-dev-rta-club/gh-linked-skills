package compat

import (
	"context"
	"fmt"
	"strings"
)

type Runner interface {
	Run(ctx context.Context, args ...string) (stdout string, stderr string, err error)
}

type Checker struct {
	runner Runner
}

func NewChecker(runner Runner) *Checker {
	return &Checker{runner: runner}
}

func (c *Checker) CheckStatus(ctx context.Context) error {
	return c.checkAvailable(ctx)
}

func (c *Checker) CheckInstall(ctx context.Context) error {
	return c.checkAvailable(ctx)
}

func (c *Checker) CheckPublish(ctx context.Context) error {
	return c.checkAvailable(ctx)
}

func (c *Checker) checkAvailable(ctx context.Context) error {
	_, stderr, err := c.runner.Run(ctx, "--version")
	if err != nil {
		return fmt.Errorf("check gh availability: %s: %w", commandDetail(stderr), err)
	}

	return nil
}

func commandDetail(stderr string) string {
	detail := strings.TrimSpace(stderr)
	if detail == "" {
		return "command failed"
	}
	return detail
}
