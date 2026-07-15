package workspace

import "github.com/game-dev-rta-club/gh-linked-skills/internal/source"

type Reader struct{}

func (Reader) Read(path string) (LocalSkill, error) {
	return ReadSkill(path)
}

type Writer struct{}

func (Writer) Install(path string, remote source.SkillSnapshot) error {
	return InstallSkill(path, remote)
}

func (Writer) ReplaceExact(path string, remote source.SkillSnapshot, expected LocalSkill, commit func() error) error {
	return ReplaceExact(path, remote, expected, commit)
}
