package source

import (
	"bytes"
	"errors"
	"fmt"
	"net/url"
	"strings"
)

var ErrSkillNotFound = errors.New("skill path not found")

type RefKind string

const (
	BranchRef RefKind = "branch"
	TagRef    RefKind = "tag"
)

type Ref struct {
	Kind     RefKind
	Name     string
	FullName string
}

type ResolvedRef struct {
	RefSHA    string
	CommitSHA string
}

func NewRef(kind RefKind, name string) (Ref, error) {
	prefix := ""
	switch kind {
	case BranchRef:
		prefix = "refs/heads/"
	case TagRef:
		prefix = "refs/tags/"
	default:
		return Ref{}, fmt.Errorf("unsupported ref kind %q", kind)
	}
	if err := validateRefName(name); err != nil {
		return Ref{}, err
	}
	return Ref{Kind: kind, Name: name, FullName: prefix + name}, nil
}

func ParseRef(value string) (Ref, error) {
	switch {
	case strings.HasPrefix(value, "refs/heads/"):
		return NewRef(BranchRef, strings.TrimPrefix(value, "refs/heads/"))
	case strings.HasPrefix(value, "refs/tags/"):
		return NewRef(TagRef, strings.TrimPrefix(value, "refs/tags/"))
	default:
		return Ref{}, fmt.Errorf("source ref must start with refs/heads/ or refs/tags/")
	}
}

func validateRefName(name string) error {
	if name == "" || name == "@" || strings.HasPrefix(name, "-") || strings.HasPrefix(name, "/") ||
		strings.HasSuffix(name, "/") || strings.HasSuffix(name, ".") || strings.Contains(name, "..") ||
		strings.Contains(name, "@{") || strings.Contains(name, "//") || strings.ContainsAny(name, "\\ ~^:?*[") {
		return fmt.Errorf("invalid source ref name %q", name)
	}
	for _, character := range name {
		if character < 0x20 || character == 0x7f {
			return fmt.Errorf("invalid source ref name %q", name)
		}
	}
	for _, component := range strings.Split(name, "/") {
		if component == "" || strings.HasPrefix(component, ".") || strings.HasSuffix(component, ".lock") {
			return fmt.Errorf("invalid source ref name %q", name)
		}
	}
	return nil
}

type Repository struct {
	Owner string
	Name  string
}

func (s SkillSnapshot) Exact(other SkillSnapshot) bool {
	if len(s.Files) != len(other.Files) || len(s.Executable) != len(other.Executable) {
		return false
	}
	for filePath, content := range s.Files {
		otherContent, ok := other.Files[filePath]
		if !ok || !bytes.Equal(content, otherContent) || s.Executable[filePath] != other.Executable[filePath] {
			return false
		}
	}
	return true
}

type SkillSnapshot struct {
	CommitSHA  string
	TreeSHA    string
	Files      map[string][]byte
	Executable map[string]bool
}

type TreeEntry struct {
	Path string
	Mode string
	Type string
	SHA  string
}

type RepositoryTree struct {
	SHA     string
	Entries []TreeEntry
}

func ParseRepository(raw string) (Repository, string) {
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Scheme != "https" || parsed.Host != "github.com" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return Repository{}, "unsupported_host"
	}
	parts := strings.Split(strings.Trim(strings.TrimSuffix(parsed.Path, ".git"), "/"), "/")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return Repository{}, "invalid_source_repository"
	}
	return Repository{Owner: parts[0], Name: parts[1]}, ""
}
