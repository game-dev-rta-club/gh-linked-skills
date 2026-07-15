package proposal

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"path"
	"regexp"
	"strings"
)

const (
	MetadataVersion = 1
	markerStart     = "<!-- gh-skill-linker:proposal"
	markerEnd       = "-->"
)

var shaPattern = regexp.MustCompile(`^(?:[0-9a-f]{40}|[0-9a-f]{64})$`)

type Metadata struct {
	Version         int    `json:"version"`
	SourcePath      string `json:"sourcePath"`
	BaseRef         string `json:"baseRef"`
	BaseTreeSHA     string `json:"baseTreeSHA"`
	ProposedTreeSHA string `json:"proposedTreeSHA"`
	HeadCommitSHA   string `json:"headCommitSHA"`
}

type State string

const (
	Waiting       State = "waiting"
	Update        State = "update"
	SourceChanged State = "source_changed"
	Obsolete      State = "obsolete"
	Diverged      State = "diverged"
	Ambiguous     State = "ambiguous"
	Unknown       State = "unknown"
)

type PullRequest struct {
	Number         int
	URL            string
	State          string
	Body           string
	Merged         bool
	HeadRef        string
	HeadSHA        string
	HeadRepository string
	BaseRef        string
	BaseRepository string
}

type ListOptions struct {
	State string
	Base  string
	Head  string
}

type CreateRequest struct {
	Title string
	Head  string
	Base  string
	Body  string
}

func SetMetadata(body string, metadata Metadata) (string, error) {
	if err := metadata.Validate(); err != nil {
		return "", err
	}
	cleanBody, err := withoutMetadata(body)
	if err != nil {
		return "", err
	}
	encoded, err := json.Marshal(metadata)
	if err != nil {
		return "", fmt.Errorf("encode proposal metadata: %w", err)
	}
	cleanBody = strings.TrimRight(cleanBody, "\n")
	if cleanBody != "" {
		cleanBody += "\n\n"
	}
	return cleanBody + markerStart + "\n" + string(encoded) + "\n" + markerEnd, nil
}

func ParseMetadata(body string) (Metadata, error) {
	start, end, err := markerBounds(body)
	if err != nil {
		return Metadata{}, err
	}
	raw := strings.TrimSpace(body[start+len(markerStart) : end])
	decoder := json.NewDecoder(strings.NewReader(raw))
	decoder.DisallowUnknownFields()
	var metadata Metadata
	if err := decoder.Decode(&metadata); err != nil {
		return Metadata{}, fmt.Errorf("decode proposal metadata: %w", err)
	}
	if err := requireJSONEOF(decoder); err != nil {
		return Metadata{}, err
	}
	if err := metadata.Validate(); err != nil {
		return Metadata{}, err
	}
	return metadata, nil
}

func (metadata Metadata) Validate() error {
	if metadata.Version != MetadataVersion {
		return fmt.Errorf("unsupported proposal metadata version %d", metadata.Version)
	}
	if err := validateRepositoryPath(metadata.SourcePath); err != nil {
		return fmt.Errorf("invalid proposal source path: %w", err)
	}
	if !strings.HasPrefix(metadata.BaseRef, "refs/heads/") || len(strings.TrimPrefix(metadata.BaseRef, "refs/heads/")) == 0 {
		return fmt.Errorf("proposal base ref must be a branch")
	}
	for name, value := range map[string]string{
		"proposed tree": metadata.ProposedTreeSHA, "head commit": metadata.HeadCommitSHA,
	} {
		if !shaPattern.MatchString(value) {
			return fmt.Errorf("invalid proposal %s SHA", name)
		}
	}
	if metadata.BaseTreeSHA != "" && !shaPattern.MatchString(metadata.BaseTreeSHA) {
		return fmt.Errorf("invalid proposal base tree SHA")
	}
	return nil
}

func Classify(localTreeSHA, baseTreeSHA, headCommitSHA string, metadata Metadata) (State, bool) {
	if baseTreeSHA == metadata.ProposedTreeSHA {
		return "", true
	}
	if headCommitSHA != metadata.HeadCommitSHA {
		return Diverged, false
	}
	if baseTreeSHA != metadata.BaseTreeSHA {
		return SourceChanged, false
	}
	if localTreeSHA == baseTreeSHA && metadata.ProposedTreeSHA != baseTreeSHA {
		return Obsolete, false
	}
	if localTreeSHA != metadata.ProposedTreeSHA {
		return Update, false
	}
	return Waiting, false
}

func BranchPrefix(skillName, sourcePath string) string {
	digest := sha256.Sum256([]byte(sourcePath))
	return "skill-linker/" + skillName + "-" + hex.EncodeToString(digest[:4])
}

func BranchName(prefix, baseTreeSHA, proposedTreeSHA string, attempt int) string {
	shortBase := "new"
	if baseTreeSHA != "" {
		shortBase = baseTreeSHA
		if len(shortBase) > 12 {
			shortBase = shortBase[:12]
		}
	}
	shortTree := proposedTreeSHA
	if len(shortTree) > 12 {
		shortTree = shortTree[:12]
	}
	name := prefix + "/" + shortBase + "-" + shortTree
	if attempt > 1 {
		name += fmt.Sprintf("-%d", attempt)
	}
	return name
}

func markerBounds(body string) (int, int, error) {
	if strings.Count(body, markerStart) != 1 {
		return 0, 0, fmt.Errorf("proposal body must contain exactly one metadata marker")
	}
	start := strings.Index(body, markerStart)
	tail := body[start+len(markerStart):]
	if strings.Count(tail, markerEnd) != 1 {
		return 0, 0, fmt.Errorf("proposal metadata marker is incomplete or ambiguous")
	}
	end := start + len(markerStart) + strings.Index(tail, markerEnd)
	return start, end, nil
}

func withoutMetadata(body string) (string, error) {
	if !strings.Contains(body, markerStart) {
		return body, nil
	}
	start, end, err := markerBounds(body)
	if err != nil {
		return "", err
	}
	return body[:start] + body[end+len(markerEnd):], nil
}

func requireJSONEOF(decoder *json.Decoder) error {
	var trailing json.RawMessage
	if err := decoder.Decode(&trailing); err == io.EOF {
		return nil
	} else if err != nil {
		return fmt.Errorf("decode proposal metadata trailing data: %w", err)
	}
	return fmt.Errorf("proposal metadata contains multiple JSON values")
}

func validateRepositoryPath(value string) error {
	if value == "" || strings.Contains(value, "\\") || strings.HasPrefix(value, "/") {
		return fmt.Errorf("path must be a relative POSIX path")
	}
	if path.Clean(value) != value || value == "." || value == ".." || strings.HasPrefix(value, "../") {
		return fmt.Errorf("path must be canonical")
	}
	for _, component := range strings.Split(value, "/") {
		if component == "" || component == "." || component == ".." {
			return fmt.Errorf("path contains an unsafe component")
		}
	}
	return nil
}
