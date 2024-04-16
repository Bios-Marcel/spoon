package windows_test

import (
	"os"
	"strings"
	"testing"

	"github.com/Bios-Marcel/spoon/internal/windows"
	"github.com/stretchr/testify/require"
)

func TestCreateJunctions(t *testing.T) {
	t.Parallel()

	if !strings.EqualFold("TRUE", os.Getenv("DESTRUCTIVE_TESTS")) {
		t.SkipNow()
	}

	t.Cleanup(func() {
		os.RemoveAll("./dir_a")
		os.RemoveAll("./dir_b")
		os.RemoveAll("./dir_a_junc")
		os.RemoveAll("./dir_b_junc")
	})

	require.NoError(t, os.MkdirAll("./dir_a", os.ModeDir))
	require.NoError(t, os.MkdirAll("./dir_b", os.ModeDir))

	require.NoError(t, windows.CreateJunctions(
		[2]string{"./dir_a", "./dir_a_junc"},
		[2]string{"./dir_b", "./dir_b_junc"},
	))
	require.FileExists(t, "./dir_a_junc")
	require.FileExists(t, "./dir_b_junc")
}

func TestCreateShortcuts(t *testing.T) {
	t.Parallel()

	if !strings.EqualFold("TRUE", os.Getenv("DESTRUCTIVE_TESTS")) {
		t.SkipNow()
	}

	t.Cleanup(func() {
		os.Remove("Le Name.lnk")
	})

	require.NoError(t, windows.CreateShortcuts(windows.Shortcut{
		Dir:        "./",
		LinkTarget: "./file.exe",
		Alias:      "Le Name",
	}))
	require.FileExists(t, "Le Name.lnk")
}
