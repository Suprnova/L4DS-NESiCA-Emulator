package wire

import "bytes"

// getString reads a NUL-terminated C string from a fixed-size field.
func getString(src []byte) string {
	if i := bytes.IndexByte(src, 0); i >= 0 {
		return string(src[:i])
	}
	return string(src)
}
