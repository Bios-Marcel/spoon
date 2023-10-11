package scoop

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

func getDirEntries(dir string) ([]fs.FileInfo, error) {
	dirHandle, err := os.Open(dir)
	if err != nil {
		return nil, err
	}

	return dirHandle.Readdir(-1)
}

type Bucket string

// Bucket is the directory name of the bucket and therefore name of the bucket.
func (b Bucket) Name() string {
	return filepath.Base(filepath.Clean(string(b)))
}

// Path is the directory path of the bucket.
func (b Bucket) Path() string {
	return string(b)
}

// AvailableApps returns unloaded app manifests. You need to call
// [App.LoadDetails] on each one. This allows for optimisation by
// parallelisation where desired.
func (b Bucket) AvailableApps() ([]App, error) {
	entries, err := getDirEntries(b.Path())
	if err != nil {
		return nil, fmt.Errorf("error getting bucket entries: %w", err)
	}

	apps := make([]App, len(entries))
	for index, entry := range entries {
		name := entry.Name()
		apps[index] = App{
			// Cut off .json
			Name:         name[:len(name)-5],
			manifestPath: filepath.Join(b.Path(), name),
		}
	}

	return apps, nil
}

// GetLocalBuckets is an API representation of locally installed buckets.
func GetLocalBuckets() ([]Bucket, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("error getting home directory: %w", err)
	}

	bucketPaths, err := filepath.Glob(filepath.Join(home, "scoop/buckets/*/bucket"))
	if err != nil {
		return nil, fmt.Errorf("error globbing buckets: %w", err)
	}

	buckets := make([]Bucket, len(bucketPaths))
	for index, bucketPath := range bucketPaths {
		buckets[index] = Bucket(bucketPath)
	}
	return buckets, nil
}

// App represents an application, which may or may not be installed and may or
// may not be part of a bucket. "Headless" manifests are also a thing, for
// example when you are using an auto-generated manifest for a version that's
// not available anymore. In that case, scoop will lose the bucket information.
type App struct {
	manifestPath string
	Name         string `json:"name"`
	Description  string `json:"description"`
	Bin          any    `json:"bin"`
	Version      string `json:"version"`
}

func (a App) ManifestPath() string {
	return a.manifestPath
}

// LoadDetails will load additional data regarding the manifest, such as
// description and version information. This causes IO on your drive and
// therefore isn't done by default.
func (a *App) LoadDetails() error {
	// We are abuising the version to indicate whether we have already loaded
	// the manifest, as the version can't be empty.
	if a.Version != "" {
		return nil
	}

	file, err := os.Open(a.manifestPath)
	if err != nil {
		return fmt.Errorf("error loading app manifest: %w", err)
	}
	defer file.Close()

	if err := json.NewDecoder(file).Decode(a); err != nil {
		return fmt.Errorf("error decoding manifest: %w", err)
	}

	return nil
}

func GetInstalledApps() ([]App, error) {
	scoopHome, err := GetAppsDir()
	if err != nil {
		return nil, fmt.Errorf("error getting scoop home directory: %w", err)
	}

	manifestPaths, err := filepath.Glob(filepath.Join(scoopHome, "*/current/manifest.json"))
	if err != nil {
		return nil, fmt.Errorf("error globbing manifests: %w", err)
	}

	apps := make([]App, len(manifestPaths))
	for index, manifestPath := range manifestPaths {
		apps[index] = App{
			Name:         strings.TrimSuffix(filepath.Base(filepath.Dir(filepath.Dir(manifestPath))), ".json"),
			manifestPath: manifestPath,
		}
	}

	return apps, nil
}

func GetScoopDir() (string, error) {
	scoopEnv := os.Getenv("SCOOP")
	if scoopEnv != "" {
		return scoopEnv, nil
	}

	// FIXME Read scoop config, as it takes precedence over fallback

	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("error getting home directory: %w", err)
	}

	return filepath.Join(home, "scoop"), nil
}

func GetAppsDir() (string, error) {
	scoopHome, err := GetScoopDir()
	if err != nil {
		return "", fmt.Errorf("error getting scoop home directory: %w", err)
	}

	return filepath.Join(scoopHome, "apps"), nil
}
