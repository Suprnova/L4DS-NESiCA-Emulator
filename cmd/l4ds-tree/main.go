//go:build windows

// Command l4ds-tree builds and patches private copies of the game ("trees"), so the installed
// game is never written to.
//
//	l4ds-tree build -game-dir <game folder> -out <new folder> [-client]
//	l4ds-tree apply <tree>
//	l4ds-tree revert <tree>
//
// build makes a server tree, which l4ds-aio runs as dedicated servers (-ds-dir), or with
// -client a client tree, a copy of the game to run with its own card (see
// gametree.ClientPatches). A server tree's srcds.exe is built from cmd/srcds-stub, so run
// build from inside the repository with Go on the PATH. apply and revert apply or restore the
// tree's patches in place; a tree with srcds.exe is a server tree.
//
// Never delete a tree with a recursive delete that follows junctions, such as Windows
// PowerShell 5.1's Remove-Item -Recurse: most of a tree is junctions to the game's folders.
package main

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/Suprnova/L4DS-NESiCA-Emulator/internal/gametree"
)

func main() {
	if len(os.Args) < 2 {
		usage()
	}

	var err error
	switch command, args := os.Args[1], os.Args[2:]; command {
	case "build":
		err = build(args)
	case "apply", "revert":
		if len(args) != 1 {
			usage()
		}
		tree := args[0]
		patches := treePatches(tree)
		if command == "apply" {
			err = gametree.Apply(tree, patches)
		} else {
			err = gametree.Revert(tree, patches)
		}
		if err == nil {
			fmt.Printf("%s: %d patches in %s\n", command, len(patches), tree)
		}
	default:
		usage()
	}

	if err != nil {
		fmt.Fprintln(os.Stderr, "l4ds-tree:", err)
		os.Exit(1)
	}
}

func build(args []string) error {
	flags := flag.NewFlagSet("build", flag.ExitOnError)
	gameDir := flags.String("game-dir", "", "installed game folder (required)")
	out := flags.String("out", "", "folder to create for the tree; it must not exist (required)")
	client := flags.Bool("client", false, "build a client tree instead of a server tree")
	flags.Parse(args)
	if *gameDir == "" || *out == "" {
		flags.Usage()
		os.Exit(2)
	}

	tree, err := filepath.Abs(*out)
	if err != nil {
		return err
	}
	patches := gametree.ServerPatches
	if *client {
		patches = gametree.ClientPatches
	}
	fmt.Println("building", tree)
	if err := gametree.Build(*gameDir, tree, patches); err != nil {
		return err
	}
	if !*client {
		if err := buildStub(tree); err != nil {
			return err
		}
	}
	fmt.Println("done")
	return nil
}

// buildStub builds cmd/srcds-stub into the tree as srcds.exe: a 32-bit GUI program, like the
// real srcds.exe.
func buildStub(tree string) error {
	cmd := exec.Command("go", "build", "-ldflags=-H=windowsgui", "-o", filepath.Join(tree, "srcds.exe"), "github.com/Suprnova/L4DS-NESiCA-Emulator/cmd/srcds-stub")
	cmd.Env = append(os.Environ(), "GOOS=windows", "GOARCH=386")
	cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("build srcds.exe (run from the repository with Go on the PATH): %w", err)
	}
	return nil
}

// treePatches returns the patches of the kind of tree at tree: server trees hold srcds.exe.
func treePatches(tree string) []gametree.Patch {
	if _, err := os.Stat(filepath.Join(tree, "srcds.exe")); err == nil {
		return gametree.ServerPatches
	}
	return gametree.ClientPatches
}

func usage() {
	fmt.Fprintln(os.Stderr, "usage:\n  l4ds-tree build -game-dir <game folder> -out <new folder> [-client]\n  l4ds-tree apply <tree>\n  l4ds-tree revert <tree>")
	os.Exit(2)
}
