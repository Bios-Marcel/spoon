package scoop

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"github.com/Bios-Marcel/spoon/internal/git"
	"github.com/Bios-Marcel/spoon/internal/windows"
	"github.com/Bios-Marcel/versioncmp"
	jsoniter "github.com/json-iterator/go"
)

type Bucket struct {
	name        string
	rootDir     string
	manifestDir string
}

// Bucket is the directory name of the bucket and therefore name of the bucket.
func (b *Bucket) Name() string {
	if b.name == "" {
		b.name = filepath.Base(filepath.Clean(b.rootDir))
	}
	return b.name
}

// Dir is the bucket directory, which contains the subdirectory "bucket" with
// the manifests.
func (b *Bucket) Dir() string {
	return b.rootDir
}

// ManifestDir is the directory path of the bucket without a leading slash.
func (b *Bucket) ManifestDir() string {
	if b.manifestDir != "" {
		return b.manifestDir
	}

	// The standard scoop buckets contain a subdirectory `bucket`, which
	// contains the manifests. However, you can also have a flat repository,
	// with just `.json` files at the top.
	defaultPath := filepath.Join(b.rootDir, "bucket")
	if _, err := os.Stat(defaultPath); !os.IsNotExist(err) {
		// We won't handle the error case here, as it is probably irrelevant
		// and hasn't handle before either.
		b.manifestDir = defaultPath
	} else {
		b.manifestDir = b.rootDir
	}

	return b.manifestDir
}

func (b *Bucket) GetApp(name string) *App {
	potentialManifest := filepath.Join(b.ManifestDir(), name+".json")
	if _, err := os.Stat(potentialManifest); err == nil {
		return &App{
			Bucket:       b,
			Name:         name,
			manifestPath: potentialManifest,
		}
	}
	return nil
}

// Remove removes the bucket, but doesn't unisntall any of its installed
// applications.
func (b *Bucket) Remove() error {
	return os.RemoveAll(b.Dir())
}

// ParseAppIdentifier returns all fragments of an app. The fragments are (in
// order) (bucket, name, version). Not that `bucket` and `version` can be empty.
func ParseAppIdentifier(name string) (string, string, string) {
	var bucket string
	if bucketSeparator := strings.IndexByte(name, '/'); bucketSeparator != -1 {
		bucket = name[:bucketSeparator]
		name = name[bucketSeparator+1:]
	}

	var version string
	if versionSeparator := strings.LastIndexByte(name, '@'); versionSeparator != -1 {
		// We don't use the version right now, so we'll just cut it off.
		version = name[versionSeparator+1:]
		name = name[:versionSeparator]
	}

	return bucket, name, version
}

var ErrBucketNotFound = errors.New("bucket not found")

// GetBucket constructs a new bucket object pointing at the given bucket. At
// this point, the bucket might not necessarily exist.
func (scoop *Scoop) GetBucket(name string) *Bucket {
	return &Bucket{rootDir: filepath.Join(scoop.GetBucketsDir(), name)}
}

func (scoop *Scoop) GetAvailableApp(name string) (*App, error) {
	bucket, name, _ := ParseAppIdentifier(name)
	if bucket != "" {
		return scoop.GetBucket(bucket).GetApp(name), nil
	}

	buckets, err := scoop.GetLocalBuckets()
	if err != nil {
		return nil, fmt.Errorf("error getting local buckets: %w", err)
	}
	for _, bucket := range buckets {
		if app := bucket.GetApp(name); app != nil {
			return app, nil
		}
	}
	return nil, nil
}

func (scoop *Scoop) GetInstalledApp(name string) (*InstalledApp, error) {
	iter := jsoniter.Parse(jsoniter.ConfigFastest, nil, 256)
	return scoop.getInstalledApp(iter, name)
}

func (scoop *Scoop) getInstalledApp(iter *jsoniter.Iterator, name string) (*InstalledApp, error) {
	_, name, _ = ParseAppIdentifier(name)
	name = strings.ToLower(name)

	appDir := filepath.Join(scoop.GetAppsDir(), name, "current")

	installJson, err := os.Open(filepath.Join(appDir, "install.json"))
	if err != nil {
		// App not installed.
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("error reading install.json: %w", err)
	}

	iter.Reset(installJson)

	var (
		bucketName string
		hold       bool
	)
	for field := iter.ReadObject(); field != ""; field = iter.ReadObject() {
		switch field {
		case "bucket":
			bucketName = iter.ReadString()
		case "hold":
			hold = iter.ReadBool()
		default:
			iter.Skip()
		}
	}

	var bucket *Bucket
	if bucketName != "" {
		bucket = scoop.GetBucket(bucketName)
	}

	return &InstalledApp{
		Hold: hold,
		App: &App{
			Bucket:       bucket,
			Name:         name,
			manifestPath: filepath.Join(appDir, "manifest.json"),
		},
	}, nil
}

// AvailableApps returns unloaded app manifests. You need to call
// [App.LoadDetails] on each one. This allows for optimisation by
// parallelisation where desired.
func (b *Bucket) AvailableApps() ([]*App, error) {
	manifestDir := b.ManifestDir()
	names, err := windows.GetDirFilenames(manifestDir)
	if err != nil {
		return nil, fmt.Errorf("error getting bucket entries: %w", err)
	}

	apps := make([]*App, 0, len(names))
	for _, name := range names {
		// Especially in flat buckets we might have other files, such as .git,
		// LICENSE or a README.md or whatever else people put there.
		if !strings.HasSuffix(name, ".json") {
			continue
		}

		apps = append(apps, &App{
			Bucket: b,
			// Cut off .json
			Name:         name[:len(name)-5],
			manifestPath: manifestDir + "\\" + name,
		})
	}

	return apps, nil
}

type KnownBucket struct {
	Name string
	URL  string
}

// GetKnownBuckets returns the list of available "default" buckets that are
// available, but might have not necessarily been installed locally.
func (scoop *Scoop) GetKnownBuckets() ([]KnownBucket, error) {
	file, err := os.Open(filepath.Join(scoop.GetScoopInstallationDir(), "buckets.json"))
	if err != nil {
		return nil, fmt.Errorf("error opening buckets.json: %w", err)
	}
	defer file.Close()

	iter := jsoniter.NewIterator(jsoniter.ConfigFastest)
	iter.Reset(file)

	var buckets []KnownBucket
	for bucketName := iter.ReadObject(); bucketName != ""; bucketName = iter.ReadObject() {
		buckets = append(buckets, KnownBucket{
			Name: bucketName,
			URL:  iter.ReadString(),
		})
	}
	return buckets, nil
}

// GetLocalBuckets is an API representation of locally installed buckets.
func (scoop *Scoop) GetLocalBuckets() ([]*Bucket, error) {
	potentialBuckets, err := windows.GetDirFilenames(scoop.GetBucketsDir())
	if err != nil {
		return nil, fmt.Errorf("error reading bucket names: %w", err)
	}

	buckets := make([]*Bucket, 0, len(potentialBuckets))
	for _, potentialBucket := range potentialBuckets {
		// While the bucket folder SHOULD only contain buckets, one could
		// accidentally place ANYTHING else in it, even textfiles.
		absBucketPath := filepath.Join(scoop.GetBucketsDir(), potentialBucket)
		file, err := os.Stat(absBucketPath)
		if err != nil {
			return nil, fmt.Errorf("error stat-ing potential bucket: %w", err)
		}
		if !file.IsDir() {
			continue
		}

		buckets = append(buckets, &Bucket{rootDir: absBucketPath})
	}
	return buckets, nil
}

// App represents an application, which may or may not be installed and may or
// may not be part of a bucket. "Headless" manifests are also a thing, for
// example when you are using an auto-generated manifest for a version that's
// not available anymore. In that case, scoop will lose the bucket information.
type App struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Version     string `json:"version"`
	Notes       string `json:"notes"`

	Bin        []Bin    `json:"bin"`
	Shortcuts  []Bin    `json:"shortcuts"`
	EnvAddPath []string `json:"env_add_path"`
	EnvSet     []EnvVar `json:"env_set"`

	Depends      []Dependency                      `json:"depends"`
	URL          []string                          `json:"url"`
	Architecture map[ArchitectureKey]*Architecture `json:"architecture"`
	InnoSetup    bool                              `json:"innosetup"`
	Installer    *Installer                        `json:"installer"`
	PreInstall   []string                          `json:"pre_install"`
	PostInstall  []string                          `json:"post_install"`
	ExtractTo    []string                          `json:"extract_to"`
	// ExtractDir specifies which dir should be extracted from the downloaded
	// archive. However, there might be more URLs than there are URLs.
	ExtractDir []string `json:"extract_dir"`

	Bucket       *Bucket `json:"-"`
	manifestPath string
}

type InstalledApp struct {
	*App
	// Hold indicates whether the app should be kept on the currently installed
	// version. It's versioning pinning.
	Hold bool
}

type OutdatedApp struct {
	*InstalledApp

	ManifestDeleted bool
	LatestVersion   string
}

type EnvVar struct {
	Key, Value string
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

type ArchitectureKey string

const (
	// Architecture32Bit is for x386 (intel/amd). It is the default if no arch
	// has been specified.
	ArchitectureKey32Bit ArchitectureKey = "32bit"
	// Architecture32Bit is for x686 (intel/amd)
	ArchitectureKey64Bit ArchitectureKey = "64bit"
	ArchitectureKeyARM64 ArchitectureKey = "arm64"
)

type Architecture struct {
	Items []ArchitectureItem `json:"items"`

	Bin       []Bin
	Shortcuts []Bin

	// Installer replaces MSI
	Installer Installer

	// PreInstall contains a list of commands to execute before installation.
	// Note that PreUninstall isn't supported in ArchitectureItem, even though
	// Uninstaller is supported.
	PreInstall []string
	// PreInstall contains a list of commands to execute after installation.
	// Note that PostUninstall isn't supported in ArchitectureItem, even though
	// Uninstaller is supported.
	PostInstall []string
}

type ArchitectureItem struct {
	URL        string
	Hash       string
	ExtractDir string
}

type Installer struct {
	// File is the installer executable. If not specified, this will
	// autoamtically be set to the last item of the URLs.
	File   string
	Script []string
	Args   []string
	Keep   bool
}

func (a App) ManifestPath() string {
	return a.manifestPath
}

const (
	DetailFieldBin          = "bin"
	DetailFieldShortcuts    = "shortcuts"
	DetailFieldUrl          = "url"
	DetailFieldArchitecture = "architecture"
	DetailFieldDescription  = "description"
	DetailFieldVersion      = "version"
	DetailFieldNotes        = "notes"
	DetailFieldDepends      = "depends"
	DetailFieldEnvSet       = "env_set"
	DetailFieldEnvAddPath   = "env_add_path"
	DetailFieldExtractDir   = "extract_dir"
	DetailFieldExtractTo    = "extract_to"
	DetailFieldPostInstall  = "post_install"
	DetailFieldPreInstall   = "pre_install"
	DetailFieldInstaller    = "installer"
	DetailFieldInnoSetup    = "innosetup"
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
		case DetailFieldUrl:
			a.URL = parseStringOrArray(iter)
		case DetailFieldShortcuts:
			a.Shortcuts = parseBin(iter)
		case DetailFieldBin:
			a.Bin = parseBin(iter)
		case DetailFieldArchitecture:
			// Preallocate to 3, as we support at max 3 architectures
			a.Architecture = make(map[ArchitectureKey]*Architecture, 3)
			for arch := iter.ReadObject(); arch != ""; arch = iter.ReadObject() {
				var archValue Architecture
				a.Architecture[ArchitectureKey(arch)] = &archValue

				var urls, hashes, extractDirs []string
				for field := iter.ReadObject(); field != ""; field = iter.ReadObject() {
					switch field {
					case "url":
						urls = parseStringOrArray(iter)
					case "hash":
						hashes = parseStringOrArray(iter)
					case "extract_dir":
						extractDirs = parseStringOrArray(iter)
					case "bin":
						archValue.Bin = parseBin(iter)
					case "shortcuts":
						archValue.Shortcuts = parseBin(iter)
					default:
						iter.Skip()
					}
				}

				// We use non-pointers, as we'll have everything initiliased
				// already then. It can happen that we have different
				// extract_dirs, but only one archive, containing both
				// architectures.
				archValue.Items = make([]ArchitectureItem, max(len(urls), len(extractDirs)))

				// We assume that we have the same length in each. While this
				// hasn't been specified in the app manifests wiki page, it's
				// the seemingly only sensible thing to me.
				for index, value := range urls {
					archValue.Items[index].URL = value
				}
				for index, value := range hashes {
					archValue.Items[index].Hash = value
				}
				for index, value := range extractDirs {
					archValue.Items[index].ExtractDir = value
				}
			}
		case DetailFieldDepends:
			// Array at top level to create multiple entries
			if iter.WhatIsNext() == jsoniter.ArrayValue {
				for iter.ReadArray() {
					a.Depends = append(a.Depends, a.parseDependency(iter.ReadString()))
				}
			} else {
				a.Depends = []Dependency{a.parseDependency(iter.ReadString())}
			}
		case DetailFieldEnvAddPath:
			a.EnvAddPath = parseStringOrArray(iter)
		case DetailFieldEnvSet:
			for key := iter.ReadObject(); key != ""; key = iter.ReadObject() {
				a.EnvSet = append(a.EnvSet, EnvVar{Key: key, Value: iter.ReadString()})
			}
		case DetailFieldInstaller:
			a.Installer = &Installer{}
			for field := iter.ReadObject(); field != ""; field = iter.ReadObject() {
				switch field {
				case "file":
					a.Installer.File = iter.ReadString()
				case "script":
					a.Installer.Script = parseStringOrArray(iter)
				case "args":
					a.Installer.Args = parseStringOrArray(iter)
				case "keep":
					a.Installer.Keep = iter.ReadBool()
				default:
					iter.Skip()
				}
			}
		case DetailFieldInnoSetup:
			a.InnoSetup = iter.ReadBool()
		case DetailFieldPreInstall:
			a.PreInstall = parseStringOrArray(iter)
		case DetailFieldPostInstall:
			a.PostInstall = parseStringOrArray(iter)
		case DetailFieldExtractDir:
			a.ExtractDir = parseStringOrArray(iter)
		case DetailFieldExtractTo:
			a.ExtractTo = parseStringOrArray(iter)
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

	if a.Installer != nil {
		if len(a.Architecture) > 0 {
			// FIXME Get Architecvhture
		} else if len(a.URL) > 0 {
			a.Installer.File = a.URL[len(a.URL)-1]
		}
	}

	return nil
}

func parseBin(iter *jsoniter.Iterator) []Bin {
	// Array at top level to create multiple entries
	if iter.WhatIsNext() == jsoniter.ArrayValue {
		var bins []Bin
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
				bins = append(bins, bin)
			} else {
				// String in the root level array to add to path
				bins = append(bins, Bin{Name: iter.ReadString()})
			}
		}
		return bins
	}

	// String value at root level to add to path.
	return []Bin{{Name: iter.ReadString()}}
}

func parseStringOrArray(iter *jsoniter.Iterator) []string {
	if iter.WhatIsNext() == jsoniter.ArrayValue {
		var val []string
		for iter.ReadArray() {
			val = append(val, iter.ReadString())
		}
		return val
	}

	return []string{iter.ReadString()}
}

func (a App) parseDependency(value string) Dependency {
	parts := strings.SplitN(value, "/", 1)
	switch len(parts) {
	case 0:
		// Should be a broken manifest
		return Dependency{}
	case 1:
		// No bucket means same bucket.
		return Dependency{Bucket: a.Bucket.Name(), Name: parts[0]}
	default:
		return Dependency{Bucket: parts[0], Name: parts[1]}
	}
}

type Dependencies struct {
	App    *App
	Values []*Dependencies
}

func (scoop *Scoop) DependencyTree(a *App) (*Dependencies, error) {
	dependencies := Dependencies{App: a}
	for _, dependency := range a.Depends {
		bucket := scoop.GetBucket(dependency.Bucket)
		dependencyApp := bucket.GetApp(dependency.Name)
		subTree, err := scoop.DependencyTree(dependencyApp)
		if err != nil {
			return nil, fmt.Errorf("error getting sub dependency tree: %w", err)
		}
		dependencies.Values = append(dependencies.Values, subTree)
	}
	return &dependencies, nil
}

func (scoop *Scoop) ReverseDependencyTree(apps []*App, app *App) *Dependencies {
	dependencies := Dependencies{App: app}
	for _, potentialDependant := range apps {
		for _, dep := range potentialDependant.Depends {
			if dep.Name == app.Name {
				subTree := scoop.ReverseDependencyTree(apps, potentialDependant)
				dependencies.Values = append(dependencies.Values, subTree)
			}
			break
		}
	}
	return &dependencies
}

func (scoop *Scoop) GetOutdatedApps() ([]*OutdatedApp, error) {
	installJSONPaths, err := filepath.Glob(filepath.Join(scoop.GetAppsDir(), "*/current/install.json"))
	if err != nil {
		return nil, fmt.Errorf("error globbing manifests: %w", err)
	}

	iter := jsoniter.Parse(jsoniter.ConfigFastest, nil, 1024*128)

	outdated := make([]*OutdatedApp, 0, len(installJSONPaths))
	for _, installJSON := range installJSONPaths {
		file, err := os.Open(installJSON)
		if err != nil {
			return nil, fmt.Errorf("error opening '%s': %w", installJSON, err)
		}
		defer file.Close()

		iter.Reset(file)

		var bucket string
		for field := iter.ReadObject(); field != ""; field = iter.ReadObject() {
			if field == "bucket" {
				bucket = iter.ReadString()
				break
			}

			iter.Skip()
		}

		appDir := filepath.Dir(filepath.Dir(installJSON))

		// Apps with autogenerated manifests lose their connection to their
		// original bucket. However, we can still search all buckets to do a
		// guess.
		appName := filepath.Base(appDir)
		if bucket != "" {
			// FIXME Add info somewhere.
			appName = bucket + "/" + appName
		}

		// We don't access the bucket directly, as this function supports
		// searching with and without bucket.
		app, err := scoop.GetAvailableApp(appName)
		if err != nil {
			return nil, fmt.Errorf("error getting app '%s' from bucket: %w", appName, err)
		}

		installedApp, err := scoop.getInstalledApp(iter, appName)
		if err != nil {
			return nil, fmt.Errorf("error getting installed app '%s': %w", appName, err)
		}

		if err := installedApp.LoadDetailsWithIter(iter, DetailFieldVersion); err != nil {
			return nil, fmt.Errorf("error loading installed app details: %w", err)
		}

		// Valid, as we can have an app installed that was deleted from the
		// bucket.
		if app != nil {
			if err := app.LoadDetailsWithIter(iter, DetailFieldVersion); err != nil {
				return nil, fmt.Errorf("error loading app details: %w", err)
			}
		}

		var latestVersion string
		if app != nil {
			latestVersion = app.Version
		}

		if versioncmp.Compare(
			installedApp.Version, latestVersion,
			versioncmp.VersionCompareRules{},
		) != "" {
			outdated = append(outdated, &OutdatedApp{
				ManifestDeleted: app == nil,
				InstalledApp:    installedApp,
				LatestVersion:   latestVersion,
			})
		}
	}
	return outdated, nil
}

func (scoop *Scoop) GetInstalledApps() ([]*InstalledApp, error) {
	manifestPaths, err := filepath.Glob(filepath.Join(scoop.GetAppsDir(), "*/current/manifest.json"))
	if err != nil {
		return nil, fmt.Errorf("error globbing manifests: %w", err)
	}

	apps := make([]*InstalledApp, len(manifestPaths))
	for index, manifestPath := range manifestPaths {
		// FIXME Check if installation stems from correct bucket!
		installJson := make(map[string]any)
		bytes, err := os.ReadFile(filepath.Join(filepath.Dir(manifestPath), "install.json"))
		if err != nil {
			return nil, fmt.Errorf("error reading install.json: %w", err)
		}
		if err := json.Unmarshal(bytes, &installJson); err != nil {
			return nil, fmt.Errorf("error unmarshalling: %w", err)
		}

		var bucket *Bucket
		bucketName := installJson["bucket"]
		if bucketNameStr, ok := bucketName.(string); ok {
			bucket = scoop.GetBucket(bucketNameStr)
		}
		apps[index] = &InstalledApp{App: &App{
			Bucket:       bucket,
			Name:         strings.TrimSuffix(filepath.Base(filepath.Dir(filepath.Dir(manifestPath))), ".json"),
			manifestPath: manifestPath,
		}}
	}

	return apps, nil
}

func (scoop *Scoop) GetBucketsDir() string {
	return filepath.Join(scoop.scoopRoot, "buckets")
}

func (scoop *Scoop) GetScoopInstallationDir() string {
	return filepath.Join(scoop.GetAppsDir(), "scoop", "current")
}

func GetDefaultScoopDir() (string, error) {
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

func (scoop *Scoop) Install(apps []string, arch ArchitectureKey) error {
	for _, inputName := range apps {
		app, err := scoop.GetAvailableApp(inputName)
		if err != nil {
			return fmt.Errorf("error installing app '%s': %w", inputName, err)
		}

		// FIXME Instead try to find it installed / history / workspace.
		// Scoop doesnt do this, but we could do it with a "dangerous" flag.
		if app == nil {
			return fmt.Errorf("app '%s' not found", inputName)
		}

		// We might be trying to install a specific version of the given
		// application. If this happens, we first look for the manifest in our
		// git history. If that fails, we try to auto-generate it. The later is
		// what scoop always does.
		_, _, version := ParseAppIdentifier(inputName)
		if version != "" {
		}
	}

	return nil
}

var ErrBucketNoGitDir = errors.New(".git dir at path not found")

func (a *App) AvailableVersions() ([]string, error) {
	repoPath, relManifestPath := git.GitPaths(a.ManifestPath())
	if repoPath == "" || relManifestPath == "" {
		return nil, ErrBucketNoGitDir
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	resultChan := make(chan git.FileContentResult, 5)
	if err := git.FileContents(ctx, repoPath, relManifestPath, resultChan); err != nil {
		return nil, fmt.Errorf("error reading file contents from git: %w", err)
	}

	iter := jsoniter.ParseBytes(jsoniter.ConfigFastest, nil)
	var versions []string
	for result := range resultChan {
		if result.Error != nil {
			return nil, result.Error
		}

		version := readVersion(iter, result.Data)
		// We could technically touch the same version in multiple commits.
		// However, we assume there should be no versions inbetween, hence we
		// only compare to the last item.
		if len(versions) == 0 || versions[len(versions)-1] != version {
			versions = append(versions, version)
		}
	}
	return versions, nil
}

func readVersion(iter *jsoniter.Iterator, data []byte) string {
	iter.ResetBytes(data)

	for field := iter.ReadObject(); field != ""; field = iter.ReadObject() {
		if field == "version" {
			return iter.ReadString()
		}
		iter.Skip()
	}
	return ""
}

// ManifestForVersion will search through history til a version equal to the
// desired version is found. Note that we compare the versions and stop
// searching if a lower version is encountered. This function is expected to
// be very slow, be warned!
func (a *App) ManifestForVersion(targetVersion string) (io.ReadCloser, error) {
	repoPath, relManifestPath := git.GitPaths(a.ManifestPath())
	if repoPath == "" || relManifestPath == "" {
		return nil, ErrBucketNoGitDir
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	resultChan := make(chan git.FileContentResult, 5)
	if err := git.FileContents(ctx, repoPath, relManifestPath, resultChan); err != nil {
		return nil, fmt.Errorf("error reading file contents from git: %w", err)
	}

	cmpRules := versioncmp.VersionCompareRules{}
	iter := jsoniter.ParseBytes(jsoniter.ConfigFastest, nil)
	for result := range resultChan {
		if result.Error != nil {
			return nil, result.Error
		}

		// We've found our file. But we open a new reader, as we
		// can't reset the existing one. Buffering it would probably not
		// be cheaper, so this is approach is fine.
		version := readVersion(iter, result.Data)
		comparison := versioncmp.Compare(version, targetVersion, cmpRules)
		if comparison == "" {
			return io.NopCloser(bytes.NewReader(result.Data)), nil
		}

		// The version we are looking for is greater than the one from history,
		// meaning we probably don't have the version in our history.
		if comparison == targetVersion {
			break
		}
	}

	// Nothing found!
	return nil, nil
}

// LookupCache will check the cache dir for matching entries. Note that the
// `app` parameter must be non-empty, but the version is optional.
func (scoop *Scoop) LookupCache(app, version string) ([]string, error) {
	expectedPrefix := cachePathRegex.ReplaceAllString(app, "_")
	if version != "" {
		expectedPrefix += "#" + cachePathRegex.ReplaceAllString(version, "_")
	}

	return filepath.Glob(filepath.Join(scoop.GetCacheDir(), expectedPrefix+"*"))
}

var cachePathRegex = regexp.MustCompile(`[^\w\.\-]+`)

// CachePath generates a path given the app, a version and the target URL. The
// rules defined here are taken from the scoop code.
func CachePath(app, version, url string) string {
	parts := []string{app, version, url}
	for i, part := range parts {
		parts[i] = cachePathRegex.ReplaceAllString(part, "_")
	}
	return strings.Join(parts, "#")
}

func (scoop *Scoop) GetCacheDir() string {
	return filepath.Join(scoop.scoopRoot, "cache")
}

type Scoop struct {
	scoopRoot string
}

func (scoop *Scoop) GetAppsDir() string {
	return filepath.Join(scoop.scoopRoot, "apps")
}

func NewScoop() (*Scoop, error) {
	dir, err := GetDefaultScoopDir()
	if err != nil {
		return nil, fmt.Errorf("error getting default scoop dir: %w", err)
	}
	return NewCustomScoop(dir), nil
}

func NewCustomScoop(scoopRoot string) *Scoop {
	return &Scoop{
		scoopRoot: scoopRoot,
	}
}
