package hotkey

import (
	"context"
	"fmt"
	"runtime"
	"unsafe"

	"golang.org/x/sys/windows"
)

const (
	noRepeat = 0x4000
	wmHotkey = 0x0312
	wmQuit   = 0x0012
)

var (
	user32            = windows.NewLazySystemDLL("user32.dll")
	registerHotKey    = user32.NewProc("RegisterHotKey")
	unregisterHotKey  = user32.NewProc("UnregisterHotKey")
	getMessage        = user32.NewProc("GetMessageW")
	postThreadMessage = user32.NewProc("PostThreadMessageW")
)

type message struct {
	window  uintptr
	message uint32
	wParam  uintptr
	lParam  uintptr
	time    uint32
	x, y    int32
	private uint32
}

// Listen registers keys system-wide and calls pressed with a key's index each time it is pressed,
// until ctx is done. It fails at once if a key cannot be registered, which happens when another
// program holds it. Holding a key down calls pressed once.
func Listen(ctx context.Context, keys []Key, pressed func(index int)) error {
	started := make(chan error, 1)
	done := make(chan struct{})
	var thread uint32
	go func() {
		defer close(done)
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()
		thread = windows.GetCurrentThreadId()
		for i, key := range keys {
			if ok, _, err := registerHotKey.Call(0, uintptr(i), uintptr(key.modifiers|noRepeat), uintptr(key.code)); ok == 0 {
				unregister(i)
				started <- fmt.Errorf("register %s: %w", key, err)
				return
			}
		}
		defer unregister(len(keys))
		started <- nil

		var msg message
		for {
			if result, _, _ := getMessage.Call(uintptr(unsafe.Pointer(&msg)), 0, 0, 0); int32(result) <= 0 {
				return
			}
			if msg.message == wmHotkey {
				pressed(int(msg.wParam))
			}
		}
	}()

	if err := <-started; err != nil {
		return err
	}
	<-ctx.Done()
	postThreadMessage.Call(uintptr(thread), wmQuit, 0, 0)
	<-done
	return nil
}

// unregister releases the first n keys of this thread.
func unregister(n int) {
	for i := range n {
		unregisterHotKey.Call(0, uintptr(i))
	}
}
