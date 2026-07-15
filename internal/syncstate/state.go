package syncstate

import "bytes"

type State string

const (
	Clean    State = "clean"
	Pull     State = "pull"
	Push     State = "push"
	Conflict State = "conflict"
)

type Snapshot map[string][]byte

func (s Snapshot) Equal(other Snapshot) bool {
	if len(s) != len(other) {
		return false
	}
	for path, content := range s {
		otherContent, ok := other[path]
		if !ok || !bytes.Equal(content, otherContent) {
			return false
		}
	}
	return true
}

func (s Snapshot) HasGeneratedConflictMarker() bool {
	markers := [][]byte{
		[]byte("<<<<<<< gh-skill-linker:local"),
		[]byte("||||||| gh-skill-linker:base:"),
		[]byte(">>>>>>> gh-skill-linker:remote:"),
	}
	for _, content := range s {
		for _, marker := range markers {
			if bytes.Contains(content, marker) {
				return true
			}
		}
	}
	return false
}

func Calculate(base, local, remote Snapshot, hasConflictMarker bool) State {
	return CalculateChanges(!local.Equal(base), !remote.Equal(base), hasConflictMarker)
}

func CalculateChanges(localChanged, remoteChanged, hasConflictMarker bool) State {
	if hasConflictMarker {
		return Conflict
	}
	switch {
	case localChanged && remoteChanged:
		return Conflict
	case localChanged:
		return Push
	case remoteChanged:
		return Pull
	default:
		return Clean
	}
}
