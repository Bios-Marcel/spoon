package scoop

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	jsoniter "github.com/json-iterator/go"
)

func getDirFilenames(dir string) ([]string, error) {
	dirHandle, err := os.Open(dir)
	if err != nil {
		return nil, err
	}
	return dirHandle.Readdirnames(-1)
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

// ManifestDir is the directory path of the bucket without a leading slash.
func (b Bucket) ManifestDir() string {
	return filepath.Join(string(b), "bucket")
}

// Remove removes the bucket, but doesn't unisntall any of its installed
// applications.
func (b Bucket) Remove() error {
	return os.RemoveAll(b.Dir())
}

func GetAvailableApp(name string) (*App, error) {
	var bucket string
	if bucketSeparator := strings.IndexByte(name, '/'); bucketSeparator != -1 {
		bucket = name[:bucketSeparator]
		name = name[bucketSeparator+1:]
	}

	versionSeparator := strings.LastIndexByte(name, '@')
	if versionSeparator != -1 {
		// We don't use the version right now, so we'll just cut it off.
		name = name[:versionSeparator]
	}

	if bucket != "" {
		return getAppFromBucket(bucket, name)
	}

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
func (b Bucket) AvailableApps() ([]*App, error) {
	manifestDir := b.ManifestDir()
	names, err := getDirFilenames(manifestDir)
	if err != nil {
		return nil, fmt.Errorf("error getting bucket entries: %w", err)
	}

	apps := make([]*App, len(names))
	for index, name := range names {
		apps[index] = &App{
			// Cut off .json
			Name:         name[:len(name)-5],
			manifestPath: manifestDir + "\\" + name,
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
	bucketPaths, err := getDirFilenames(bucketsPath)
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
	Bin          []Bin        `json:"bin"`
	Depends      []Dependency `json:"depends"`
}

type Dependency struct {
	Bucket string
	Name   string
}

type Bin struct {
	Name  string
	Alias string
	Args  []string
}

func (a App) ManifestPath() string {
	return a.manifestPath
}

func (a App) Bucket() string {
	return filepath.Base(filepath.Dir(a.manifestPath))
}

const (
	DetailFieldBin         = "bin"
	DetailFieldDescription = "description"
	DetailFieldVersion     = "version"
	DetailFieldNotes       = "notes"
	DetailFieldDepends     = "depends"
)

// LoadDetails will load additional data regarding the manifest, such as
// description and version information. This causes IO on your drive and
// therefore isn't done by default.
func (a *App) LoadDetails(fields ...string) error {
	iter := jsoniter.Parse(jsoniter.ConfigFastest, nil, 1024*128)
	return a.LoadDetailsWithIter(iter, fields...)
}

// LoadDetails will load additional data regarding the manifest, such as
// description and version information. This causes IO on your drive and
// therefore isn't done by default.
func (a *App) LoadDetailsWithIter(iter *jsoniter.Iterator, fields ...string) error {
	file, err := os.Open(a.manifestPath)
	if err != nil {
		return fmt.Errorf("error opening manifest: %w", err)
	}
	defer file.Close()

	iter.Reset(file)

	for field := iter.ReadObject(); field != ""; field = iter.ReadObject() {
		if !slices.Contains(fields, field) {
			iter.Skip()
			continue
		}

		switch field {
		case DetailFieldDescription:
			a.Description = iter.ReadString()
		case DetailFieldVersion:
			a.Version = iter.ReadString()
		case DetailFieldBin:
			// Array at top level to create multiple entries
			if iter.WhatIsNext() == jsoniter.ArrayValue {
				for iter.ReadArray() {
					// There are nested arrays, for shim creation, with format:
					// binary alias [args...]
					if iter.WhatIsNext() == jsoniter.ArrayValue {
						var bin Bin
						if iter.ReadArray() {
							bin.Name = iter.ReadString()
						}
						if iter.ReadArray() {
							bin.Alias = iter.ReadString()
						}
						for iter.ReadArray() {
							bin.Args = append(bin.Args, iter.ReadString())
						}
						a.Bin = append(a.Bin, bin)
					} else {
						// String in the root level array to add to path
						a.Bin = append(a.Bin, Bin{Name: iter.ReadString()})
					}
				}
			} else {
				// String vaue at root level to add to path.
				a.Bin = []Bin{{Name: iter.ReadString()}}
			}
		case DetailFieldDepends:
			// Array at top level to create multiple entries
			if iter.WhatIsNext() == jsoniter.ArrayValue {
				for iter.ReadArray() {
					a.Depends = append(a.Depends, a.parseDependency(iter.ReadString()))
				}
			} else {
				// String vaue at root level to add to path.
				a.Depends = []Dependency{a.parseDependency(iter.ReadString())}
			}
		case DetailFieldNotes:
			if iter.WhatIsNext() == jsoniter.ArrayValue {
				var lines []string
				for iter.ReadArray() {
					lines = append(lines, iter.ReadString())
				}
				a.Notes = strings.Join(lines, "\n")
			} else {
				a.Notes = iter.ReadString()
			}
		default:
			iter.Skip()
		}
	}

	if iter.Error != nil {
		return fmt.Errorf("error parsing json: %w", iter.Error)
	}

	return nil
}

func (a App) parseDependency(value string) Dependency {
	parts := strings.SplitN(value, "/", 1)
	switch len(parts) {
	case 0:
		// Should be a broken manifest
		return Dependency{}
	case 1:
		// No bucket means same bucket.
		return Dependency{Bucket: a.Bucket(), Name: parts[0]}
	default:
		return Dependency{Bucket: parts[0], Name: parts[1]}
	}
}

type Dependencies struct {
	App    *App
	Values []*Dependencies
}

func getAppFromBucket(bucket, app string) (*App, error) {
	bucketDir, err := GetScoopBucketDir()
	if err != nil {
		return nil, fmt.Errorf("error getting bucket dir: %w", err)
	}

	return &App{
		Name:         app,
		manifestPath: filepath.Join(bucketDir, bucket, "bucket", app+".json"),
	}, nil
}

func (a App) DependencyTree() (*Dependencies, error) {
	dependencies := Dependencies{App: &a}
	for _, dependency := range a.Depends {
		dependencyApp, err := getAppFromBucket(dependency.Bucket, dependency.Name)
		if err != nil {
			return nil, fmt.Errorf("error getting info about dependency: %w", err)
		}

		subTree, err := dependencyApp.DependencyTree()
		if err != nil {
			return nil, fmt.Errorf("error getting sub dependency tree: %w", err)
		}
		dependencies.Values = append(dependencies.Values, subTree)
	}
	return &dependencies, nil
}

func (a App) ReverseDependencyTree(apps []*App) *Dependencies {
	dependencies := Dependencies{App: &a}
	for _, app := range apps {
		for _, dep := range app.Depends {
			if dep.Name == a.Name {
				subTree := app.ReverseDependencyTree(apps)
				dependencies.Values = append(dependencies.Values, subTree)
			}
			break
		}
	}
	return &dependencies
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
