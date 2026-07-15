package command

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"sort"
	"strings"
)

type Runner struct {
	executable  string
	environment map[string]string
	secrets     []string
}

func New(executable string) Runner {
	return Runner{executable: executable}
}

func NewWithEnv(executable string, environment map[string]string, secrets ...string) Runner {
	copy := make(map[string]string, len(environment))
	for key, value := range environment {
		copy[key] = value
	}
	for key, value := range map[string]string{
		"GIT_TRACE":             "0",
		"GIT_TRACE_CURL":        "0",
		"GIT_TRACE_PACKET":      "0",
		"GIT_TRACE_REDACT":      "1",
		"GIT_CURL_VERBOSE":      "0",
		"GIT_TRACE2":            "0",
		"GIT_TRACE2_EVENT":      "0",
		"GIT_TRACE2_PERF":       "0",
		"GIT_TRACE_SETUP":       "0",
		"GIT_TRACE_SHALLOW":     "0",
		"GIT_TRACE_PERFORMANCE": "0",
	} {
		copy[key] = value
	}
	return Runner{executable: executable, environment: copy, secrets: append([]string(nil), secrets...)}
}

func (r Runner) Run(ctx context.Context, args ...string) (string, string, error) {
	command := exec.CommandContext(ctx, r.executable, args...)
	if len(r.environment) > 0 {
		values := make(map[string]string)
		for _, entry := range os.Environ() {
			key, value, ok := strings.Cut(entry, "=")
			if ok {
				values[key] = value
			}
		}
		for key, value := range r.environment {
			values[key] = value
		}
		keys := make([]string, 0, len(values))
		for key := range values {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		command.Env = make([]string, 0, len(keys))
		for _, key := range keys {
			command.Env = append(command.Env, key+"="+values[key])
		}
	}
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	command.Stdout = &stdout
	command.Stderr = &stderr
	err := command.Run()
	return r.redact(stdout.String()), r.redact(stderr.String()), err
}

func (r Runner) redact(value string) string {
	for _, secret := range r.secrets {
		if secret != "" {
			value = strings.ReplaceAll(value, secret, "[REDACTED]")
		}
	}
	return value
}
