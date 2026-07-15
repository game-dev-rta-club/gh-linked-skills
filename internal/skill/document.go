package skill

import (
	"bytes"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"unicode/utf8"

	"gopkg.in/yaml.v3"
)

var ErrInvalidDocument = errors.New("invalid skill document")

var skillNamePattern = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`)

func ParseName(content []byte) (string, error) {
	frontmatter, err := splitFrontmatter(content)
	if err != nil {
		return "", err
	}
	var header struct {
		Name        string `yaml:"name"`
		Description string `yaml:"description"`
	}
	if err := yaml.Unmarshal(frontmatter, &header); err != nil {
		return "", fmt.Errorf("%w: parse YAML frontmatter: %v", ErrInvalidDocument, err)
	}
	if len(header.Name) == 0 || len(header.Name) > 64 || !skillNamePattern.MatchString(header.Name) {
		return "", fmt.Errorf("%w: invalid skill name", ErrInvalidDocument)
	}
	if len(strings.TrimSpace(header.Description)) == 0 || utf8.RuneCountInString(header.Description) > 1024 {
		return "", fmt.Errorf("%w: invalid skill description", ErrInvalidDocument)
	}
	return header.Name, nil
}

func splitFrontmatter(content []byte) ([]byte, error) {
	if !bytes.HasPrefix(content, []byte("---\n")) && !bytes.HasPrefix(content, []byte("---\r\n")) {
		return nil, fmt.Errorf("%w: missing opening frontmatter delimiter", ErrInvalidDocument)
	}
	lineStart := bytes.IndexByte(content, '\n') + 1
	frontmatterStart := lineStart
	for lineStart < len(content) {
		lineEnd := bytes.IndexByte(content[lineStart:], '\n')
		if lineEnd < 0 {
			lineEnd = len(content)
		} else {
			lineEnd += lineStart
		}
		line := bytes.TrimSuffix(content[lineStart:lineEnd], []byte{'\r'})
		if bytes.Equal(line, []byte("---")) {
			return content[frontmatterStart:lineStart], nil
		}
		if lineEnd == len(content) {
			break
		}
		lineStart = lineEnd + 1
	}
	return nil, fmt.Errorf("%w: missing closing frontmatter delimiter", ErrInvalidDocument)
}
