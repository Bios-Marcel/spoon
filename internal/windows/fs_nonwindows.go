//go:build !windows

package windows

import "os"

// IsLink reports whether path is a symbolic link.
func IsLink(path string) (bool, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return false, err
	}
	return info.Mode()&os.ModeSymlink != 0, nil
}
