//go:build windows

package windows_test

import (
	"os"
	"path/filepath"
	"syscall"
	"testing"

	"github.com/Bios-Marcel/spoon/internal/windows"
	"github.com/stretchr/testify/require"
)

func TestForceRemoveAllBestEffortContinuesAfterLockedFile(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	lockedPath := filepath.Join(dir, "a-locked")
	removablePath := filepath.Join(dir, "z-removable")
	require.NoError(t, os.WriteFile(lockedPath, nil, 0o600))
	require.NoError(t, os.WriteFile(removablePath, nil, 0o600))

	lockedPathPtr, err := syscall.UTF16PtrFromString(lockedPath)
	require.NoError(t, err)
	handle, err := syscall.CreateFile(
		lockedPathPtr,
		syscall.GENERIC_READ,
		0, // In particular, do not share deletion access.
		nil,
		syscall.OPEN_EXISTING,
		syscall.FILE_ATTRIBUTE_NORMAL,
		0,
	)
	require.NoError(t, err)
	defer syscall.CloseHandle(handle)

	err = windows.ForceRemoveAllBestEffort(dir)
	require.Error(t, err)
	require.FileExists(t, lockedPath)
	require.NoFileExists(t, removablePath)
}
