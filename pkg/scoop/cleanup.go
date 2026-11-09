package scoop

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/Bios-Marcel/spoon/internal/windows"
)

// CleanupOptions controls the optional cache cleanup performed alongside an
// application's old installed versions.
type CleanupOptions struct {
	RemoveCache bool
	// BeforeRemoveVersion is called immediately before an old installed version
	// is removed. Callers can use it to display progress for slow removals.
	BeforeRemoveVersion func(version string)
	// CacheDir can be set when cleaning a global installation, whose downloads
	// are still stored in the current user's cache. An empty value uses the
	// Scoop instance's cache directory.
	CacheDir string
}

// CleanupResult describes an application cleanup attempt.
type CleanupResult struct {
	CurrentVersion  string
	RemovedVersions []string
}

// Cleanup removes every installed version of app except the selected current
// version. Unlike Scoop, it attempts every removal and joins any errors only
// after the remaining versions and cache entries have been processed.
func (scoop *Scoop) Cleanup(app string, options CleanupOptions) (CleanupResult, error) {
	if app == "" || app == "." || app == ".." || filepath.Base(app) != app ||
		strings.ContainsAny(app, `/\\`) {
		return CleanupResult{}, fmt.Errorf("invalid app name %q", app)
	}
	return scoop.cleanup(app, options, windows.ForceRemoveAllBestEffort)
}

func (scoop *Scoop) cleanup(
	app string,
	options CleanupOptions,
	remove func(string) error,
) (CleanupResult, error) {
	appDir := filepath.Join(scoop.AppDir(), app)
	currentVersion, err := selectCurrentVersion(appDir)
	if err != nil {
		return CleanupResult{}, fmt.Errorf("error selecting current version of %q: %w", app, err)
	}

	result := CleanupResult{CurrentVersion: currentVersion}
	var errs []error

	if options.RemoveCache {
		cacheDir := options.CacheDir
		if cacheDir == "" {
			cacheDir = scoop.CacheDir()
		}
		if err := cleanupAppCache(cacheDir, app, currentVersion, remove); err != nil {
			errs = append(errs, err)
		}
	}

	entries, err := os.ReadDir(appDir)
	if err != nil {
		errs = append(errs, fmt.Errorf("error reading installed versions of %q: %w", app, err))
		return result, errors.Join(errs...)
	}

	for _, entry := range entries {
		version := entry.Name()
		if strings.EqualFold(version, currentVersion) || strings.EqualFold(version, "current") {
			continue
		}

		result.RemovedVersions = append(result.RemovedVersions, version)
		if options.BeforeRemoveVersion != nil {
			options.BeforeRemoveVersion(version)
		}
		if err := remove(filepath.Join(appDir, version)); err != nil {
			errs = append(errs, fmt.Errorf("error removing %s %s: %w", app, version, err))
		}
	}

	// If an installation has no actual versions left, Scoop also removes its
	// now-dangling current junction and the application directory.
	left, err := os.ReadDir(appDir)
	if err != nil {
		if !os.IsNotExist(err) {
			errs = append(errs, fmt.Errorf("error checking remaining versions of %q: %w", app, err))
		}
		return result, errors.Join(errs...)
	}
	if len(left) == 1 && strings.EqualFold(left[0].Name(), "current") {
		currentDir := filepath.Join(appDir, "current")
		isLink, linkErr := windows.IsLink(currentDir)
		if linkErr != nil {
			errs = append(errs, fmt.Errorf("error checking current link of %q: %w", app, linkErr))
		} else if isLink {
			if err := remove(currentDir); err != nil {
				errs = append(errs, fmt.Errorf("error removing current link of %q: %w", app, err))
			}
			left, err = os.ReadDir(appDir)
			if err != nil && !os.IsNotExist(err) {
				errs = append(errs, fmt.Errorf("error checking application directory of %q: %w", app, err))
			}
		}
	}
	if len(left) == 0 {
		if err := remove(appDir); err != nil {
			errs = append(errs, fmt.Errorf("error removing empty application directory of %q: %w", app, err))
		}
	}

	return result, errors.Join(errs...)
}

func cleanupAppCache(
	cacheDir, app, currentVersion string,
	remove func(string) error,
) error {
	entries, err := os.ReadDir(cacheDir)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("error reading cache for %q: %w", app, err)
	}

	appPrefix := strings.ToLower(app + "#")
	currentPrefix := strings.ToLower(app + "#" + currentVersion + "#")
	var errs []error
	for _, entry := range entries {
		name := strings.ToLower(entry.Name())
		if !strings.HasPrefix(name, appPrefix) || strings.HasPrefix(name, currentPrefix) {
			continue
		}
		if err := remove(filepath.Join(cacheDir, entry.Name())); err != nil {
			errs = append(errs, fmt.Errorf("error removing cached download %q: %w", entry.Name(), err))
		}
	}
	return errors.Join(errs...)
}

// CleanupIncompleteDownloads removes Scoop's incomplete *.download files and
// attempts all matching entries before returning any errors.
func CleanupIncompleteDownloads(cacheDir string) error {
	entries, err := os.ReadDir(cacheDir)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("error reading cache: %w", err)
	}

	var errs []error
	for _, entry := range entries {
		if !strings.HasSuffix(strings.ToLower(entry.Name()), ".download") {
			continue
		}
		path := filepath.Join(cacheDir, entry.Name())
		if err := windows.ForceRemoveAllBestEffort(path); err != nil {
			errs = append(errs, fmt.Errorf("error removing incomplete download %q: %w", entry.Name(), err))
		}
	}
	return errors.Join(errs...)
}

// InstalledAppNames returns application directory names, including incomplete
// installations, just as Scoop's cleanup --all does.
func (scoop *Scoop) InstalledAppNames() ([]string, error) {
	entries, err := os.ReadDir(scoop.AppDir())
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("error reading installed applications: %w", err)
	}

	apps := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() && !strings.EqualFold(entry.Name(), "scoop") {
			apps = append(apps, entry.Name())
		}
	}
	return apps, nil
}

func selectCurrentVersion(appDir string) (string, error) {
	currentDir := filepath.Join(appDir, "current")
	manifestPaths := []string{
		filepath.Join(currentDir, "scoop-manifest.json"),
		filepath.Join(currentDir, "manifest.json"),
	}
	for _, manifestPath := range manifestPaths {
		manifest, err := os.Open(manifestPath)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return "", fmt.Errorf("error opening current manifest: %w", err)
		}

		var current struct {
			Version string `json:"version"`
		}
		decodeErr := json.NewDecoder(manifest).Decode(&current)
		closeErr := manifest.Close()
		if decodeErr != nil {
			return "", fmt.Errorf("error parsing current manifest: %w", decodeErr)
		}
		if closeErr != nil {
			return "", fmt.Errorf("error closing current manifest: %w", closeErr)
		}

		if !strings.EqualFold(current.Version, "nightly") {
			if current.Version != "" {
				return current.Version, nil
			}
			break
		}

		isLink, err := windows.IsLink(currentDir)
		if err == nil && isLink {
			resolved, err := filepath.EvalSymlinks(currentDir)
			if err == nil {
				return filepath.Base(resolved), nil
			}
		}
		break
	}

	return latestInstalledVersion(appDir)
}

type installedVersion struct {
	name      string
	installed time.Time
}

func latestInstalledVersion(appDir string) (string, error) {
	entries, err := os.ReadDir(appDir)
	if os.IsNotExist(err) {
		return "", nil
	}
	if err != nil {
		return "", err
	}

	versions := make([]installedVersion, 0, len(entries))
	for _, entry := range entries {
		name := entry.Name()
		lowerName := strings.ToLower(name)
		if !entry.IsDir() || lowerName == "current" ||
			(strings.HasPrefix(lowerName, "_") && strings.Contains(lowerName, ".old")) {
			continue
		}

		var installTime time.Time
		for _, installFile := range []string{"scoop-install.json", "install.json"} {
			info, err := os.Stat(filepath.Join(appDir, name, installFile))
			if os.IsNotExist(err) {
				continue
			}
			if err != nil {
				return "", err
			}
			if installTime.IsZero() || info.ModTime().Before(installTime) {
				installTime = info.ModTime()
			}
		}
		if !installTime.IsZero() {
			versions = append(versions, installedVersion{name: name, installed: installTime})
		}
	}

	sort.SliceStable(versions, func(i, j int) bool {
		return versions[i].installed.Before(versions[j].installed)
	})
	if len(versions) == 0 {
		return "", nil
	}
	return versions[len(versions)-1].name, nil
}
