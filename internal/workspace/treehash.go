package workspace

import (
	"bytes"
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"path"
	"sort"
	"strconv"
	"strings"
)

type treeFile struct {
	content    []byte
	executable bool
}

type treeNode struct {
	files map[string]treeFile
	dirs  map[string]*treeNode
}

type treeEntry struct {
	name  string
	mode  string
	hash  [sha1.Size]byte
	isDir bool
}

// TreeSHA returns the Git tree object SHA for a regular-file skill snapshot.
func TreeSHA(files map[string][]byte, executable map[string]bool) (string, error) {
	root := newTreeNode()
	for relative, content := range files {
		if err := addTreeFile(root, relative, content, executable[relative]); err != nil {
			return "", err
		}
	}
	hash := hashTree(root)
	return hex.EncodeToString(hash[:]), nil
}

func newTreeNode() *treeNode {
	return &treeNode{files: make(map[string]treeFile), dirs: make(map[string]*treeNode)}
}

func addTreeFile(root *treeNode, relative string, content []byte, executable bool) error {
	if relative == "" || strings.Contains(relative, "\\") || strings.ContainsRune(relative, 0) ||
		path.IsAbs(relative) || relative == ".." || strings.HasPrefix(relative, "../") || path.Clean(relative) != relative {
		return fmt.Errorf("unsafe tree path %q", relative)
	}
	parts := strings.Split(relative, "/")
	node := root
	for _, component := range parts[:len(parts)-1] {
		if component == "" {
			return fmt.Errorf("unsafe tree path %q", relative)
		}
		if _, exists := node.files[component]; exists {
			return fmt.Errorf("tree path %q conflicts with a file", relative)
		}
		child, exists := node.dirs[component]
		if !exists {
			child = newTreeNode()
			node.dirs[component] = child
		}
		node = child
	}
	name := parts[len(parts)-1]
	if name == "" {
		return fmt.Errorf("unsafe tree path %q", relative)
	}
	if _, exists := node.dirs[name]; exists {
		return fmt.Errorf("tree path %q conflicts with a directory", relative)
	}
	node.files[name] = treeFile{content: content, executable: executable}
	return nil
}

func hashTree(node *treeNode) [sha1.Size]byte {
	entries := make([]treeEntry, 0, len(node.files)+len(node.dirs))
	for name, file := range node.files {
		mode := "100644"
		if file.executable {
			mode = "100755"
		}
		entries = append(entries, treeEntry{name: name, mode: mode, hash: hashObject("blob", file.content)})
	}
	for name, child := range node.dirs {
		entries = append(entries, treeEntry{name: name, mode: "40000", hash: hashTree(child), isDir: true})
	}
	sort.Slice(entries, func(i, j int) bool {
		return bytes.Compare(treeSortKey(entries[i]), treeSortKey(entries[j])) < 0
	})

	var content bytes.Buffer
	for _, entry := range entries {
		content.WriteString(entry.mode)
		content.WriteByte(' ')
		content.WriteString(entry.name)
		content.WriteByte(0)
		content.Write(entry.hash[:])
	}
	return hashObject("tree", content.Bytes())
}

func treeSortKey(entry treeEntry) []byte {
	suffix := byte(0)
	if entry.isDir {
		suffix = '/'
	}
	return append(append([]byte(nil), entry.name...), suffix)
}

func hashObject(kind string, content []byte) [sha1.Size]byte {
	header := kind + " " + strconv.Itoa(len(content)) + "\x00"
	object := make([]byte, 0, len(header)+len(content))
	object = append(object, header...)
	object = append(object, content...)
	return sha1.Sum(object)
}
