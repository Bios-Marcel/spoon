package scoop

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestCleanupRemovesOldVersionsAndCache(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	testScoop := NewCustomScoop(root)
	appDir := filepath.Join(testScoop.AppDir(), "demo")
	require.NoError(t, os.MkdirAll(filepath.Join(appDir, "1.0"), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(appDir, "2.0"), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(appDir, "current"), 0o755))
	require.NoError(t, os.WriteFile(
		filepath.Join(appDir, "current", "manifest.json"),
		[]byte(`{"version":"2.0"}`), 0o600,
	))
	require.NoError(t, os.MkdirAll(testScoop.CacheDir(), 0o755))
	oldCache := filepath.Join(testScoop.CacheDir(), "demo#1.0#old.zip")
	currentCache := filepath.Join(testScoop.CacheDir(), "demo#2.0#current.zip")
	require.NoError(t, os.WriteFile(oldCache, nil, 0o600))
	require.NoError(t, os.WriteFile(currentCache, nil, 0o600))

	result, err := testScoop.Cleanup("demo", CleanupOptions{RemoveCache: true})
	require.NoError(t, err)
	require.Equal(t, "2.0", result.CurrentVersion)
	require.Equal(t, []string{"1.0"}, result.RemovedVersions)
	require.NoDirExists(t, filepath.Join(appDir, "1.0"))
	require.DirExists(t, filepath.Join(appDir, "2.0"))
	require.DirExists(t, filepath.Join(appDir, "current"))
	require.NoFileExists(t, oldCache)
	require.FileExists(t, currentCache)
}

func TestCleanupContinuesAfterRemovalFailure(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	testScoop := NewCustomScoop(root)
	appDir := filepath.Join(testScoop.AppDir(), "demo")
	for _, version := range []string{"1.0", "2.0", "3.0", "current"} {
		require.NoError(t, os.MkdirAll(filepath.Join(appDir, version), 0o755))
	}
	require.NoError(t, os.WriteFile(
		filepath.Join(appDir, "current", "manifest.json"),
		[]byte(`{"version":"2.0"}`), 0o600,
	))
	require.NoError(t, os.MkdirAll(testScoop.CacheDir(), 0o755))
	oldCache := filepath.Join(testScoop.CacheDir(), "demo#1.0#old.zip")
	require.NoError(t, os.WriteFile(oldCache, nil, 0o600))

	deleteFailure := errors.New("file is in use")
	var attempts []string
	var reportedVersions []string
	result, err := testScoop.cleanup("demo", CleanupOptions{
		RemoveCache: true,
		BeforeRemoveVersion: func(version string) {
			reportedVersions = append(reportedVersions, version)
		},
	}, func(path string) error {
		attempts = append(attempts, path)
		if path == filepath.Join(appDir, "1.0") {
			return deleteFailure
		}
		return os.RemoveAll(path)
	})

	require.ErrorIs(t, err, deleteFailure)
	require.Equal(t, []string{"1.0", "3.0"}, result.RemovedVersions)
	require.Equal(t, []string{"1.0", "3.0"}, reportedVersions)
	require.Contains(t, attempts, filepath.Join(appDir, "1.0"))
	require.Contains(t, attempts, filepath.Join(appDir, "3.0"))
	require.Contains(t, attempts, oldCache)
	require.DirExists(t, filepath.Join(appDir, "1.0"))
	require.NoDirExists(t, filepath.Join(appDir, "3.0"))
	require.NoFileExists(t, oldCache)
}

func TestCleanupSelectsLatestInstalledVersionWithoutCurrentLink(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	testScoop := NewCustomScoop(root)
	appDir := filepath.Join(testScoop.AppDir(), "demo")
	oldInstall := filepath.Join(appDir, "1.0", "install.json")
	currentInstall := filepath.Join(appDir, "2.0", "install.json")
	require.NoError(t, os.MkdirAll(filepath.Dir(oldInstall), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Dir(currentInstall), 0o755))
	require.NoError(t, os.WriteFile(oldInstall, []byte("{}"), 0o600))
	require.NoError(t, os.WriteFile(currentInstall, []byte("{}"), 0o600))
	now := time.Now()
	require.NoError(t, os.Chtimes(oldInstall, now.Add(-time.Hour), now.Add(-time.Hour)))
	require.NoError(t, os.Chtimes(currentInstall, now, now))

	result, err := testScoop.Cleanup("demo", CleanupOptions{})
	require.NoError(t, err)
	require.Equal(t, "2.0", result.CurrentVersion)
	require.Equal(t, []string{"1.0"}, result.RemovedVersions)
	require.NoDirExists(t, filepath.Join(appDir, "1.0"))
	require.DirExists(t, filepath.Join(appDir, "2.0"))
}

func TestInstalledAppNamesIncludesIncompleteInstalls(t *testing.T) {
	t.Parallel()

	testScoop := NewCustomScoop(t.TempDir())
	require.NoError(t, os.MkdirAll(filepath.Join(testScoop.AppDir(), "demo"), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(testScoop.AppDir(), "broken"), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(testScoop.AppDir(), "scoop"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(testScoop.AppDir(), "not-an-app"), nil, 0o600))

	apps, err := testScoop.InstalledAppNames()
	require.NoError(t, err)
	require.True(t, slices.Equal([]string{"broken", "demo"}, apps))
}
