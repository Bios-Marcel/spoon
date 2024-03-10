package scoop

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"github.com/buger/jsonparser"
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

// Dir is the bucket directory, which contains the subdirectory "bucket" with
// the manifests.
func (b Bucket) Dir() string {
	return string(b)
}

// ManifestDir is the directory path of the bucket.
func (b Bucket) ManifestDir() string {
	return filepath.Join(string(b), "bucket")
}

// Remove removes the bucket, but doesn't unisntall any of its installed
// applications.
func (b Bucket) Remove() error {
	return os.RemoveAll(b.Dir())
}

func GetApp(name string) (*App, error) {
	app, err := GetAvailableApp(name)
	if err != nil {
		return nil, fmt.Errorf("error getting installed app: %w", err)
	}
	if app != nil {
		return app, nil
	}
	return GetInstalledApp(name)
}

func GetAvailableApp(name string) (*App, error) {
	buckets, err := GetLocalBuckets()
	if err != nil {
		return nil, fmt.Errorf("error getting local buckets: %w", err)
	}
	for _, bucket := range buckets {
		// Since we are on windows, this is case insensitive.
		potentialManifest := filepath.Join(bucket.ManifestDir(), name+".json")
		if _, err := os.Stat(potentialManifest); err == nil {
			return &App{
				Name:         name,
				manifestPath: potentialManifest,
			}, nil
		}
	}
	return nil, nil
}

func GetInstalledApp(name string) (*App, error) {
	apps, err := GetInstalledApps()
	if err != nil {
		return nil, fmt.Errorf("error getting installed apps: %w", err)
	}
	for _, app := range apps {
		if strings.EqualFold(app.Name, name) {
			return &app, nil
		}
	}
	return nil, nil
}

// AvailableApps returns unloaded app manifests. You need to call
// [App.LoadDetails] on each one. This allows for optimisation by
// parallelisation where desired.
func (b Bucket) AvailableApps() ([]App, error) {
	entries, err := getDirEntries(b.ManifestDir())
	if err != nil {
		return nil, fmt.Errorf("error getting bucket entries: %w", err)
	}

	manifestDir := b.ManifestDir()
	buffer := make([]byte, 0, 1024)

	apps := make([]App, len(entries))
	for index, entry := range entries {
		name := entry.Name()

		buffer = buffer[:len(manifestDir)+1+len(name)]
		copy(buffer, manifestDir)
		buffer[len(manifestDir)] = '/'
		copy(buffer[len(manifestDir)+1:], name)

		apps[index] = App{
			// Cut off .json
			Name:         name[:len(name)-5],
			manifestPath: string(buffer),
		}
	}

	return apps, nil
}

// GetKnownBuckets returns the list of available "default" buckets that are
// available, but might have not necessarily been installed locally.
func GetKnownBuckets() (map[string]string, error) {
	knownBuckets := make(map[string]string)
	scoopInstallationDir, err := GetScoopInstallationDir()
	if err != nil {
		return nil, fmt.Errorf("error getting scoop installation directory: %w", err)
	}

	file, err := os.Open(filepath.Join(scoopInstallationDir, "buckets.json"))
	if err != nil {
		return nil, fmt.Errorf("error opening buckets.json: %w", err)
	}
	defer file.Close()

	if err := json.NewDecoder(file).Decode(&knownBuckets); err != nil {
		return nil, fmt.Errorf("error decoding buckets.json: %w", err)
	}

	return knownBuckets, nil
}

// GetLocalBuckets is an API representation of locally installed buckets.
func GetLocalBuckets() ([]Bucket, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("error getting home directory: %w", err)
	}

	bucketsPath := filepath.Join(home, "scoop/buckets")
	bucketsDir, err := os.Open(bucketsPath)
	if err != nil {
		return nil, fmt.Errorf("error reading buckets dir: %w", err)
	}
	defer bucketsDir.Close()

	bucketPaths, err := bucketsDir.Readdirnames(-1)
	if err != nil {
		return nil, fmt.Errorf("error reaeding bucket names: %w", err)
	}

	buckets := make([]Bucket, len(bucketPaths))
	for index, bucketPath := range bucketPaths {
		buckets[index] = Bucket(filepath.Join(bucketsPath, bucketPath))
	}
	return buckets, nil
}

// App represents an application, which may or may not be installed and may or
// may not be part of a bucket. "Headless" manifests are also a thing, for
// example when you are using an auto-generated manifest for a version that's
// not available anymore. In that case, scoop will lose the bucket information.
type App struct {
	Name         string `json:"name"`
	Description  string `json:"description"`
	Version      string `json:"version"`
	Notes        string `json:"notes"`
	manifestPath string
	Bin          []string `json:"bin"`
	loaded       bool
}

func (a App) ManifestPath() string {
	return a.manifestPath
}

const (
	DetailFieldBin         = "bin"
	DetailFieldDescription = "description"
	DetailFieldVersion     = "version"
	DetailFieldNotes       = "notes"
)

// LoadDetails will load additional data regarding the manifest, such as
// description and version information. This causes IO on your drive and
// therefore isn't done by default.
func (a *App) LoadDetails(buffer *bytes.Buffer, fields ...string) error {
	if a.loaded {
		return nil
	}

	file, err := os.Open(a.manifestPath)
	if err != nil {
		return fmt.Errorf("error opening app manifest: %w", err)
	}
	defer file.Close()

	buffer.Reset()
	_, err = io.Copy(buffer, file)
	if err != nil {
		return fmt.Errorf("error reading app manifest: %w", err)
	}

	err = jsonparser.ObjectEach(buffer.Bytes(), func(key []byte, value []byte, dataType jsonparser.ValueType, _ int) error {
		field := string(key)
		if !slices.Contains(fields, field) {
			return nil
		}

		switch field {
		case DetailFieldDescription:
			a.Description = string(value)
		case DetailFieldVersion:
			a.Version = string(value)
		case DetailFieldBin:
			if dataType == jsonparser.String {
				a.Bin = []string{string(value)}
			} else if dataType == jsonparser.Array {
				_, err := jsonparser.ArrayEach(value, func(value []byte, _ jsonparser.ValueType, _ int, err error) {
					if err != nil {
						return
					}
					a.Bin = append(a.Bin, string(value))
				})
				if err != nil {
					return fmt.Errorf("error parsing bin array: %w", err)
				}
			}
		case DetailFieldNotes:
			a.Notes = string(value)
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("error parsing app manifest: %w", err)
	}

	a.loaded = true
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

func GetScoopBucketDir() (string, error) {
	scoopHome, err := GetScoopDir()
	if err != nil {
		return "", fmt.Errorf("error getting scoop home directory: %w", err)
	}

	return filepath.Join(scoopHome, "buckets"), nil
}

func GetScoopInstallationDir() (string, error) {
	appsDir, err := GetAppsDir()
	if err != nil {
		return "", fmt.Errorf("error getting scoop apps directory: %w", err)
	}

	return filepath.Join(appsDir, "scoop", "current"), nil
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

// cachePathRegex applies the same rules to the name components as scoop does.
var cachePathRegex = regexp.MustCompile(`[^\w\.\-]+`)

func CachePath(app, version, url string) string {
	parts := []string{app, version, url}
	for i, part := range parts {
		parts[i] = cachePathRegex.ReplaceAllString(part, "_")
	}
	return strings.Join(parts, "#")
}

func GetCacheDir() (string, error) {
	scoopHome, err := GetScoopDir()
	if err != nil {
		return "", fmt.Errorf("error getting scoop home directory: %w", err)
	}

	return filepath.Join(scoopHome, "cache"), nil
}

func GetAppsDir() (string, error) {
	scoopHome, err := GetScoopDir()
	if err != nil {
		return "", fmt.Errorf("error getting scoop home directory: %w", err)
	}

	return filepath.Join(scoopHome, "apps"), nil
}
