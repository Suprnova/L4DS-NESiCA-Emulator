//go:build windows

// Command srcds-stub stands in for srcds.exe, which the arcade install lacks: it loads
// bin\dedicated.dll from a server tree and calls its DedicatedMain entry point with the arguments
// it was given, so the arcade build runs as a dedicated server.
//
// It must be built as a 32-bit GUI program, like the real srcds.exe:
//
//	GOOS=windows GOARCH=386 go build -ldflags=-H=windowsgui ./cmd/srcds-stub
//
// With -console, dedicated.dll calls AllocConsole, which fails in a process that already has a
// console. l4ds-tree builds it into each server tree.
//
// The tree is the L4DS_DIR environment variable. Setting L4DS_HIDE_CONSOLE hides the server's
// console window.
package main

import (
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"
	"unsafe"
)

func main() {
	treeDir := os.Getenv("L4DS_DIR")
	if treeDir == "" {
		panic("L4DS_DIR is not set")
	}
	binDir := filepath.Join(treeDir, "bin")
	if err := os.Chdir(treeDir); err != nil {
		panic(err)
	}
	os.Setenv("PATH", binDir+";"+os.Getenv("PATH"))

	clearStdHandles()
	if os.Getenv("L4DS_HIDE_CONSOLE") != "" {
		go hideConsoleWhenCreated()
	}

	library, err := syscall.LoadLibrary(filepath.Join(binDir, "dedicated.dll"))
	if err != nil {
		panic(err)
	}
	entry, err := syscall.GetProcAddress(library, "DedicatedMain")
	if err != nil {
		panic(err)
	}

	instance, _, _ := syscall.NewLazyDLL("kernel32.dll").NewProc("GetModuleHandleW").Call(0)
	commandLine, _ := syscall.BytePtrFromString(strings.Join(os.Args[1:], " "))
	const showNormal = 1
	syscall.SyscallN(entry, instance, 0, uintptr(unsafe.Pointer(commandLine)), showNormal)
}

// hideConsoleWhenCreated keeps the console window dedicated.dll creates with -console hidden.
// AllocConsole ignores the show state the parent asked for, and the console host shows the
// window after it exists, so it is hidden whenever it is visible.
func hideConsoleWhenCreated() {
	getConsoleWindow := syscall.NewLazyDLL("kernel32.dll").NewProc("GetConsoleWindow")
	user32 := syscall.NewLazyDLL("user32.dll")
	isWindowVisible, showWindow := user32.NewProc("IsWindowVisible"), user32.NewProc("ShowWindow")
	const hide = 0
	for {
		if window, _, _ := getConsoleWindow.Call(); window != 0 {
			if visible, _, _ := isWindowVisible.Call(window); visible != 0 {
				showWindow.Call(window, hide)
			}
		}
		time.Sleep(50 * time.Millisecond)
	}
}

// clearStdHandles empties the standard handles a parent passed in, such as the NUL handles Go's
// os/exec passes. AllocConsole only points empty standard handles at the console it creates,
// and the engine's console reads input through the standard input handle; with a NUL handle
// there, its first read fails with "CTextConsoleWin32::GetLine: !GetNumberOfConsoleInputEvents".
func clearStdHandles() {
	setStdHandle := syscall.NewLazyDLL("kernel32.dll").NewProc("SetStdHandle")
	for _, handle := range []int32{syscall.STD_INPUT_HANDLE, syscall.STD_OUTPUT_HANDLE, syscall.STD_ERROR_HANDLE} {
		setStdHandle.Call(uintptr(handle), 0)
	}
}
