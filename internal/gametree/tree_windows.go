package gametree

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
)

// writableFolders are the folders of left4dead2 the engine writes to; a tree gets copies of them.
var writableFolders = []string{"cfg", "logs", "downloads", "save", "screenshots"}

// ErrExists reports that a tree's folder already exists; Build never writes into one.
var ErrExists = errors.New("gametree: folder already exists")

// Build creates a tree at out from the game installed at gameDir and applies patches to it. bin,
// the loose files and the folders the engine writes are copies; every other folder is a
// junction to the game's, so the tree is small and the game itself is never written to.
//
// Never delete a tree with a recursive delete that follows junctions (Windows PowerShell 5.1's
// Remove-Item -Recurse does): it would delete the game's files through them.
func Build(gameDir, out string, patches []Patch) error {
	if _, err := os.Lstat(out); err == nil {
		return fmt.Errorf("%w: %s", ErrExists, out)
	}
	if err := os.MkdirAll(out, 0o755); err != nil {
		return err
	}

	entries, err := os.ReadDir(gameDir)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		src, dst := filepath.Join(gameDir, entry.Name()), filepath.Join(out, entry.Name())
		switch {
		case !entry.IsDir():
			err = copyFile(src, dst)
		case entry.Name() == "bin":
			err = os.CopyFS(dst, os.DirFS(src))
		case entry.Name() == "left4dead2":
			err = buildGameFolder(src, dst)
		default:
			err = junction(dst, src)
		}
		if err != nil {
			return err
		}
	}
	return Apply(out, patches)
}

// buildGameFolder fills a tree's left4dead2 folder: copies of its files and writable folders,
// junctions for the rest.
func buildGameFolder(src, dst string) error {
	if err := os.Mkdir(dst, 0o755); err != nil {
		return err
	}
	entries, err := os.ReadDir(src)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		from, to := filepath.Join(src, entry.Name()), filepath.Join(dst, entry.Name())
		switch {
		case !entry.IsDir():
			err = copyFile(from, to)
		case slices.Contains(writableFolders, entry.Name()):
			err = os.CopyFS(to, os.DirFS(from))
		default:
			err = junction(to, from)
		}
		if err != nil {
			return err
		}
	}
	return nil
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}

// junction makes link a directory junction to target. Unlike a symbolic link, a junction needs
// no privilege.
func junction(link, target string) error {
	if output, err := exec.Command("cmd", "/c", "mklink", "/J", link, target).CombinedOutput(); err != nil {
		return fmt.Errorf("mklink %s: %w: %s", link, err, output)
	}
	return nil
}
