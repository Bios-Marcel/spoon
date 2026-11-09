//go:build windows

package windows

import (
	"os"
	"syscall"
)

// IsLink reports whether path is a symbolic link or another Windows reparse
// point, including the directory junctions used by Scoop.
func IsLink(path string) (bool, error) {
	pathPtr, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return false, err
	}

	attributes, err := syscall.GetFileAttributes(pathPtr)
	if err != nil {
		return false, &os.PathError{Op: "GetFileAttributes", Path: path, Err: err}
	}
	return attributes&syscall.FILE_ATTRIBUTE_REPARSE_POINT != 0, nil
}
