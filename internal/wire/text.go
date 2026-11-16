package wire

import "bytes"

// putString copies s into dst as a NUL-padded fixed-size C string, truncating it so a
// terminating NUL always remains.
func putString(dst []byte, s string) {
	n := copy(dst[:len(dst)-1], s)
	clear(dst[n:])
}

// getString reads a NUL-terminated C string from a fixed-size field.
func getString(src []byte) string {
	if i := bytes.IndexByte(src, 0); i >= 0 {
		return string(src[:i])
	}
	return string(src)
}
