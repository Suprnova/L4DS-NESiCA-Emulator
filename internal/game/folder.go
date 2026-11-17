package game

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// Folder is what the server reads from an installed game: the tables that decide player
// levels, the versus classes and the stage list.
type Folder struct {
	Levels        *LevelTable
	VersusClasses []VersusClass
	Stages        []Stage
}

// LoadFolder reads the level table, the versus class table and the stage map table, with the
// stages' Japanese names, from the game installed at dir.
func LoadFolder(dir string) (Folder, error) {
	resource := filepath.Join(dir, "left4dead2", "resource")
	var folder Folder
	tokens := map[string]string{}

	steps := []struct {
		path string
		read func(io.Reader) error
	}{
		{filepath.Join(resource, "LevelTable.txt"), func(r io.Reader) (err error) {
			folder.Levels, err = ParseLevelTable(r)
			return err
		}},
		{filepath.Join(resource, "VersusClassDataTable.txt"), func(r io.Reader) (err error) {
			folder.VersusClasses, err = ParseVersusClasses(r)
			return err
		}},
		// The stage names, in the reverse of the order the game searches its folders, so the
		// update folder's text wins.
		{filepath.Join(resource, "l4d360ui_japanese.txt"), func(r io.Reader) error { return ParseTokens(r, tokens) }},
		{filepath.Join(dir, "update", "resource", "l4d360ui_tu_japanese.txt"), func(r io.Reader) error { return ParseTokens(r, tokens) }},
		{filepath.Join(resource, "stage_solo.txt"), func(r io.Reader) (err error) {
			folder.Stages, err = ParseStages(r, tokens)
			return err
		}},
	}
	for _, step := range steps {
		if err := readFile(step.path, step.read); err != nil {
			return Folder{}, err
		}
	}
	return folder, nil
}

// readFile opens the file at path for read, and names the file in read's error.
func readFile(path string, read func(io.Reader) error) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()
	if err := read(file); err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}
	return nil
}
