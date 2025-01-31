package scoop

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/md5"
	"crypto/sha1"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/hex"
	stdJson "encoding/json"
	"errors"
	"fmt"
	"hash"
	"io"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/Bios-Marcel/spoon/internal/git"
	"github.com/Bios-Marcel/spoon/internal/json"
	"github.com/Bios-Marcel/spoon/internal/windows"
	"github.com/Bios-Marcel/versioncmp"
	"github.com/cavaliergopher/grab/v3"
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

func (b *Bucket) FindApp(name string) *App {
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
	return &Bucket{rootDir: filepath.Join(scoop.BucketDir(), name)}
}

func (scoop *Scoop) FindAvailableApp(name string) (*App, error) {
	bucket, name, _ := ParseAppIdentifier(name)
	if bucket != "" {
		return scoop.GetBucket(bucket).FindApp(name), nil
	}

	buckets, err := scoop.GetLocalBuckets()
	if err != nil {
		return nil, fmt.Errorf("error getting local buckets: %w", err)
	}
	for _, bucket := range buckets {
		if app := bucket.FindApp(name); app != nil {
			return app, nil
		}
	}
	return nil, nil
}

func (scoop *Scoop) FindInstalledApp(name string) (*InstalledApp, error) {
	iter := jsoniter.Parse(jsoniter.ConfigFastest, nil, 256)
	return scoop.findInstalledApp(iter, name)
}

func (scoop *Scoop) findInstalledApp(iter *jsoniter.Iterator, name string) (*InstalledApp, error) {
	_, name, _ = ParseAppIdentifier(name)
	name = strings.ToLower(name)

	appDir := filepath.Join(scoop.AppDir(), name, "current")

	installJson, err := os.Open(filepath.Join(appDir, "install.json"))
	if err != nil {
		// App not installed.
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("error reading install.json: %w", err)
	}
	defer installJson.Close()

	json.Reset(iter, installJson)

	var (
		bucketName   string
		architecture string
		hold         bool
	)
	for field := iter.ReadObject(); field != ""; field = iter.ReadObject() {
		switch field {
		case "architecture":
			architecture = iter.ReadString()
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
		Hold:         hold,
		Architecture: ArchitectureKey(architecture),
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
			manifestPath: manifestDir + "/" + name,
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
	file, err := os.Open(filepath.Join(scoop.ScoopInstallationDir(), "buckets.json"))
	if err != nil {
		return nil, fmt.Errorf("error opening buckets.json: %w", err)
	}
	defer file.Close()

	iter := jsoniter.Parse(jsoniter.ConfigFastest, file, 1024)

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
	potentialBuckets, err := windows.GetDirFilenames(scoop.BucketDir())
	if err != nil {
		return nil, fmt.Errorf("error reading bucket names: %w", err)
	}

	buckets := make([]*Bucket, 0, len(potentialBuckets))
	for _, potentialBucket := range potentialBuckets {
		// While the bucket folder SHOULD only contain buckets, one could
		// accidentally place ANYTHING else in it, even textfiles.
		absBucketPath := filepath.Join(scoop.BucketDir(), potentialBucket)
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
//
// Note that this structure doesn't reflect the same schema as the scoop
// manifests, as we are trying to make usage easier, not just as hard.
type App struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Version     string `json:"version"`
	Notes       string `json:"notes"`

	Bin        []Bin        `json:"bin"`
	Shortcuts  []Shortcut   `json:"shortcuts"`
	EnvAddPath []string     `json:"env_add_path"`
	EnvSet     []EnvVar     `json:"env_set"`
	Persist    []PersistDir `json:"persist"`

	Downloadables []Downloadable `json:"downloadables"`

	Depends      []Dependency                      `json:"depends"`
	Architecture map[ArchitectureKey]*Architecture `json:"architecture"`
	InnoSetup    bool                              `json:"innosetup"`
	// Installer deprecates msi
	Installer     *Installer   `json:"installer"`
	Uninstaller   *Uninstaller `json:"uninstaller"`
	PreInstall    []string     `json:"pre_install"`
	PostInstall   []string     `json:"post_install"`
	PreUninstall  []string     `json:"pre_uninstall"`
	PostUninstall []string     `json:"post_uninstall"`
	ExtractTo     []string     `json:"extract_to"`

	// Spoon "internals"

	Bucket       *Bucket `json:"-"`
	manifestPath string
}

type InstalledApp struct {
	*App
	// Hold indicates whether the app should be kept on the currently installed
	// version. It's versioning pinning.
	Hold bool
	// Archictecture defines which architecture was used for installation. On a
	// 64Bit system for example, this could also be 32Bit, but not vice versa.
	Architecture ArchitectureKey
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

// PersistDir represents a directory in the installation of the application,
// which the app or the user will write to. This is placed in a separate
// location and kept upon uninstallation.
type PersistDir struct {
	Dir string
	// LinkName is optional and can be used to rename the [Dir].
	LinkName string
}

type Bin struct {
	Name  string
	Alias string
	Args  []string
}

type Shortcut struct {
	Name         string
	ShortcutName string
	Args         string
	Icon         string
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
	Downloadables []Downloadable `json:"items"`

	Bin       []Bin
	Shortcuts []Shortcut

	// Installer replaces MSI
	Installer   *Installer
	Uninstaller *Uninstaller

	// PreInstall contains a list of commands to execute before installation.
	// Note that PreUninstall isn't supported in ArchitectureItem, even though
	// Uninstaller is supported.
	PreInstall []string
	// PreInstall contains a list of commands to execute after installation.
	// Note that PostUninstall isn't supported in ArchitectureItem, even though
	// Uninstaller is supported.
	PostInstall []string
}

type Downloadable struct {
	URL  string
	Hash string
	// ExtractDir specifies which dir should be extracted from the downloaded
	// archive. However, there might be more URLs than there are ExtractDirs.
	ExtractDir string
	ExtractTo  string
}

type Installer struct {
	// File is the installer executable. If not specified, this will
	// automatically be set to the last item of the URLs. Note, that this will
	// be looked up in the extracted dirs, if explicitly specified.
	File   string
	Script []string
	Args   []string
	Keep   bool
}

type Uninstaller Installer

// invoke will run the installer script or file. This method is implemented on a
// non-pointer as we manipulate the script.
func (installer Installer) invoke(scoop *Scoop, dir string, arch ArchitectureKey) error {
	// File and Script are mutually exclusive and Keep is only used if script is
	// not set. However, we automatically set file to the last downloaded file
	// if none is set, we then pass this to the script if any is present.
	if len(installer.Script) > 0 {
		variableSubstitutions := map[string]string{
			"$fname":        installer.File,
			"$dir":          dir,
			"$architecture": string(arch),
			// FIXME We don't intend to support writing back the manifest into
			// our context for now, as it seems only 1 or 2 apps actually do
			// this. Instead, we should try to prepend a line that parses the
			// manifest inline and creates the variable locally.
			"$manifest": "TODO",
		}
		for index, line := range installer.Script {
			installer.Script[index] = substituteVariables(line, variableSubstitutions)
		}
		if err := scoop.runScript(installer.Script); err != nil {
			return fmt.Errorf("error running installer: %w", err)
		}
	} else if installer.File != "" {
		// FIXME RUN! Not extract?

		if !installer.Keep {
			// FIXME Okay ... it seems scoop downloads the files not only into
			// cache, but also into the installation directory. This seems a bit
			// wasteful to me. Instead, we should copy the files into the dir
			// only if we actually want to keep them. This way we can prevent
			// useless copy and remove actions.
			//
			// This implementation shouldn't be part of the download, but
			// instead be done during installation, manually checking both
			// uninstaller.keep and installer.keep, copying if necessary and
			// correctly invoking with the resulting paths.
		}
	}

	return nil
}

func (a *App) ManifestPath() string {
	return a.manifestPath
}

type Dependencies struct {
	App    *App
	Values []*Dependencies
}

func (scoop *Scoop) DependencyTree(a *App) (*Dependencies, error) {
	dependencies := Dependencies{App: a}
	for _, dependency := range a.Depends {
		bucket := scoop.GetBucket(dependency.Bucket)
		dependencyApp := bucket.FindApp(dependency.Name)
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
	installJSONPaths, err := filepath.Glob(filepath.Join(scoop.AppDir(), "*/current/install.json"))
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

		json.Reset(iter, file)

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
		app, err := scoop.FindAvailableApp(appName)
		if err != nil {
			return nil, fmt.Errorf("error getting app '%s' from bucket: %w", appName, err)
		}

		installedApp, err := scoop.findInstalledApp(iter, appName)
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

func (scoop *Scoop) InstalledApps() ([]*InstalledApp, error) {
	manifestPaths, err := filepath.Glob(filepath.Join(scoop.AppDir(), "*/current/manifest.json"))
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
		if err := stdJson.Unmarshal(bytes, &installJson); err != nil {
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

func (scoop *Scoop) BucketDir() string {
	return filepath.Join(scoop.scoopRoot, "buckets")
}

func (scoop *Scoop) PersistDir() string {
	return filepath.Join(scoop.scoopRoot, "persist")
}

func (scoop *Scoop) ScoopInstallationDir() string {
	return filepath.Join(scoop.AppDir(), "scoop", "current")
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

func (scoop *Scoop) runScript(lines []string) error {
	// To slash, so we don't have to escape
	bucketsDir := `"` + filepath.ToSlash(scoop.BucketDir()) + `"`

	substitutedLines := make([]string, len(lines))
	for index, line := range lines {
		substitutedLines[index] = substituteVariables(line, map[string]string{
			"$bucketsdir": bucketsDir,
		})
	}

	return windows.RunPowershellScript(substitutedLines, true)
}

// InstallAll will install the given application into userspace. If an app is
// already installed, it will be updated if applicable.
//
// One key difference to scoop however, is how installing a concrete version
// works. Instead of creating a dirty manifest, we will search for the old
// manifest, install it and hold the app. This will have the same effect for the
// user, but without the fact that the user will never again get update
// notifications.
func (scoop *Scoop) InstallAll(appNames []string, arch ArchitectureKey) []error {
	iter := manifestIter()

	var errs []error
	for _, inputName := range appNames {
		if err := scoop.install(iter, inputName, arch); err != nil {
			errs = append(errs, fmt.Errorf("error installing '%s': %w", inputName, err))
		}
	}

	return errs
}

type CacheHit struct {
	Downloadable *Downloadable
}

type FinishedDownload struct {
	Downloadable *Downloadable
}

type StartedDownload struct {
	Downloadable *Downloadable
}

type ChecksumMismatchError struct {
	Expected string
	Actual   string
	File     string
}

func (err *ChecksumMismatchError) Error() string {
	return fmt.Sprintf("checksum mismatch (%s != %s)", err.Expected, err.Actual)
}

// Download will download all files for the desired architecture, skipping
// already cached files. The cache lookups happen before downloading and are
// synchronous, directly returning an error instead of using the error channel.
// As soon as download starts (chan, chan, nil) is returned. Both channels are
// closed upon completion (success / failure).
// FIXME Make single result chan with a types:
// (download_start, download_finished, cache_hit)
func (resolvedApp *AppResolved) Download(
	cacheDir string,
	arch ArchitectureKey,
	verifyHashes, overwriteCache bool,
) (chan any, error) {
	var download []Downloadable

	// We use a channel for this, as its gonna get more once we finish download
	// packages. For downloads, this is not the case, so it is a slice.
	results := make(chan any, len(resolvedApp.Downloadables))

	if overwriteCache {
		for _, item := range resolvedApp.Downloadables {
			download = append(download, item)
		}
	} else {
		// Parallelise extraction / download. We want to make installation as fast
		// as possible.
		for _, item := range resolvedApp.Downloadables {
			path := filepath.Join(
				cacheDir,
				CachePath(resolvedApp.Name, resolvedApp.Version, item.URL),
			)
			_, err := os.Stat(path)
			if err != nil {
				if os.IsNotExist(err) {
					download = append(download, item)
					continue
				}

				close(results)
				return nil, fmt.Errorf("error checking cached file: %w", err)
			}

			if err := validateHash(path, item.Hash); err != nil {
				// FIXME We have an error here, but we'll swallow and
				// redownload. Should we possibly make a new type?
				download = append(download, item)
			} else {
				results <- &CacheHit{&item}
			}
		}
	}

	cachePath := func(downloadable Downloadable) string {
		return filepath.Join(cacheDir, CachePath(resolvedApp.Name, resolvedApp.Version, downloadable.URL))
	}
	var requests []*grab.Request
	for index, item := range download {
		request, err := grab.NewRequest(cachePath(item), item.URL)
		if err != nil {
			close(results)
			return nil, fmt.Errorf("error preparing download: %w", err)
		}

		// We attach the item as a context value, since we'll have to make a
		// separate mapping otherwise. This is a bit un-nice, but it is stable.
		request = request.WithContext(context.WithValue(context.Background(), "item", item))
		request.Label = strconv.FormatInt(int64(index), 10)
		requests = append(requests, request)
	}

	if len(requests) == 0 {
		close(results)
		return results, nil
	}

	// FIXME Determine batchsize?
	client := grab.NewClient()
	responses := client.DoBatch(2, requests...)

	// We work on multiple requests at once, but only have one extraction
	// routine, as extraction should already make use of many CPU cores.
	go func() {
		for response := range responses {
			if err := response.Err(); err != nil {
				results <- fmt.Errorf("error during download: %w", err)
				continue
			}

			downloadable := response.Request.Context().Value("item").(Downloadable)
			results <- &StartedDownload{&downloadable}

			if hashVal := downloadable.Hash; hashVal != "" && verifyHashes {
				if err := validateHash(cachePath(downloadable), hashVal); err != nil {
					results <- err
					continue
				}
			}

			results <- &FinishedDownload{&downloadable}
		}

		close(results)
	}()

	return results, nil
}

func validateHash(path, hashVal string) error {
	if hashVal == "" {
		return nil
	}

	var algo hash.Hash
	if strings.HasPrefix(hashVal, "sha1:") {
		hashVal = hashVal[5:]
		algo = sha1.New()
	} else if strings.HasPrefix(hashVal, "sha512:") {
		hashVal = hashVal[7:]
		algo = sha512.New()
	} else if strings.HasPrefix(hashVal, "md5:") {
		hashVal = hashVal[4:]
		algo = md5.New()
	} else {
		// sha256 is the default in scoop and has no prefix. This
		// will most likely not break, due to the fact scoop goes
		// hard on backwards compatibility / not having to migrate
		// any of the existing manifests.
		algo = sha256.New()
	}

	file, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("error determining checksum: %w", err)
	}
	defer file.Close()

	if _, err := io.Copy(algo, file); err != nil {
		return fmt.Errorf("error determining checksum: %w", err)
	}

	hashVal = strings.ToLower(hashVal)
	formattedHash := strings.ToLower(hex.EncodeToString(algo.Sum(nil)))

	if formattedHash != hashVal {
		return &ChecksumMismatchError{
			Actual:   formattedHash,
			Expected: hashVal,
			File:     path,
		}
	}

	return nil
}

func (scoop *Scoop) Install(appName string, arch ArchitectureKey) error {
	return scoop.install(manifestIter(), appName, arch)
}

func (scoop *Scoop) Uninstall(app *InstalledApp, arch ArchitectureKey) error {
	resolvedApp := app.ForArch(arch)

	if err := scoop.runScript(resolvedApp.PreUninstall); err != nil {
		return fmt.Errorf("error executing pre_uninstall script: %w", err)
	}

	if uninstaller := resolvedApp.Uninstaller; uninstaller != nil {
		dir := filepath.Join(scoop.AppDir(), app.Name, app.Version)
		if err := Installer(*uninstaller).invoke(scoop, dir, arch); err != nil {
			return fmt.Errorf("error invoking uninstaller: %w", err)
		}
	}

	var updatedEnvVars [][2]string
	for _, envVar := range resolvedApp.EnvSet {
		updatedEnvVars = append(updatedEnvVars, [2]string{envVar.Key, ""})
	}

	if len(resolvedApp.EnvAddPath) > 0 {
		pathKey, pathVar, err := windows.GetPersistentEnvValue("User")
		if err != nil {
			return fmt.Errorf("error retrieving path variable: %w", err)
		}

		newPath := windows.ParsePath(pathVar).Remove(resolvedApp.EnvAddPath...)
		updatedEnvVars = append(updatedEnvVars, [2]string{pathKey, newPath.String()})
	}

	if err := windows.SetPersistentEnvValues(updatedEnvVars...); err != nil {
		return fmt.Errorf("error restoring environment variables: %w", err)
	}

	appDir := filepath.Join(scoop.AppDir(), app.Name)
	currentDir := filepath.Join(appDir, "current")

	// The install dir is marked as read-only, but not the files inside.
	// This will also unlink any persist-dir.
	if err := windows.ForceRemoveAll(currentDir); err != nil {
		return fmt.Errorf("error deleting installation files: %w", err)
	}

	if err := scoop.RemoveShims(resolvedApp.Bin...); err != nil {
		return fmt.Errorf("error removing shim: %w", err)
	}

	// FIXME Should we instead manually delete all .lnk files and then check for
	// leftover empty directories? This could be relevant if there are some dirs
	// that share a shortcut subdirectory.
	if len(resolvedApp.Shortcuts) > 0 {
		startmenuPath, err := scoop.ShortcutDir()
		if err != nil {
			return err
		}

		for _, shortcut := range resolvedApp.Shortcuts {
			dir := filepath.Dir(shortcut.ShortcutName)
			if dir == "." {
				if err := os.Remove(filepath.Join(startmenuPath, shortcut.ShortcutName+".lnk")); err != nil {
					return fmt.Errorf("error deleting shortcut: %w", err)
				}
				continue
			}

			if err := os.RemoveAll(filepath.Join(startmenuPath, dir)); err != nil {
				return fmt.Errorf("error deleting shortcut dir: %w", err)
			}
		}
	}

	if err := scoop.runScript(resolvedApp.PostUninstall); err != nil {
		return fmt.Errorf("error executing post_uninstall script: %w", err)
	}
	return nil
}

var (
	ErrAlreadyInstalled         = errors.New("app already installed (same version)")
	ErrAppNotFound              = errors.New("app not found")
	ErrAppNotAvailableInVersion = errors.New("app not available in desird version")
)

func (scoop *Scoop) install(iter *jsoniter.Iterator, appName string, arch ArchitectureKey) error {
	fmt.Printf("Installing '%s' ...\n", appName)

	// FIXME Should we check installed first? If it's already installed, we can
	// just ignore if it doesn't exist in the bucket anymore.

	app, err := scoop.FindAvailableApp(appName)
	if err != nil {
		return err
	}

	// FIXME Instead try to find it installed / history / workspace.
	// Scoop doesnt do this, but we could do it with a "dangerous" flag.
	if app == nil {
		return ErrAppNotFound
	}

	installedApp, err := scoop.FindInstalledApp(appName)
	if err != nil {
		return fmt.Errorf("error checking for installed version: %w", err)
	}

	// FIXME Make force flag.
	// FIXME Should this be part of the low level install?
	if installedApp != nil && installedApp.Hold {
		return fmt.Errorf("app is held: %w", err)
	}

	// We might be trying to install a specific version of the given
	// application. If this happens, we first look for the manifest in our
	// git history. If that fails, we try to auto-generate it. The later is
	// what scoop always does.
	var manifestFile io.ReadSeeker
	_, _, version := ParseAppIdentifier(appName)
	if version != "" {
		fmt.Printf("Search for manifest version '%s' ...\n", version)
		manifestFile, err = app.ManifestForVersion(version)
		if err != nil {
			return fmt.Errorf("error finding app in version: %w", err)
		}
		if manifestFile == nil {
			return ErrAppNotAvailableInVersion
		}

		app = &App{
			Name:   app.Name,
			Bucket: app.Bucket,
		}
		if err := app.loadDetailFromManifestWithIter(iter, manifestFile, DetailFieldsAll...); err != nil {
			return fmt.Errorf("error loading manifest: %w", err)
		}
	} else {
		manifestFile, err = os.Open(app.ManifestPath())
		if err != nil {
			return fmt.Errorf("error opening manifest for copying: %w", err)
		}
		if err := app.loadDetailFromManifestWithIter(iter, manifestFile, DetailFieldsAll...); err != nil {
			return fmt.Errorf("error loading manifest: %w", err)
		}
	}

	if closer, ok := manifestFile.(io.Closer); ok {
		defer closer.Close()
	}

	// We reuse the handle.
	if _, err := manifestFile.Seek(0, 0); err != nil {
		return fmt.Errorf("error resetting manifest file handle: %w", err)
	}

	if installedApp != nil {
		if err := installedApp.LoadDetailsWithIter(iter,
			DetailFieldVersion,
			DetailFieldPreUninstall,
			DetailFieldPostUninstall,
		); err != nil {
			return fmt.Errorf("error determining installed version: %w", err)
		}

		// The user should manually run uninstall and install to reinstall.
		if installedApp.Version == app.Version && installedApp.Architecture == arch {
			return ErrAlreadyInstalled
		}

		// We use the installedApp Architecture, as it doesn't necessarily has
		// to match with the desired arch.
		if err := scoop.Uninstall(installedApp, installedApp.Architecture); err != nil {
			return fmt.Errorf("error uninstalling exiting version: %w", err)
		}
	}

	// FIXME Check if an old version is already installed and we can
	// just-relink it.

	resolvedApp := app.ForArch(arch)

	scoop.runScript(resolvedApp.PreInstall)

	appDir := filepath.Join(scoop.AppDir(), app.Name)
	versionDir := filepath.Join(appDir, app.Version)
	if err := os.MkdirAll(versionDir, os.ModeDir); err != nil {
		return fmt.Errorf("error creating installation target dir: %w", err)
	}

	cacheDir := scoop.CacheDir()
	donwloadResults, err := resolvedApp.Download(cacheDir, arch, true, false)
	if err != nil {
		return fmt.Errorf("error initialising download: %w", err)
	}

	for result := range donwloadResults {
		switch result := result.(type) {
		case error:
			return result
		case *CacheHit:
			fmt.Printf("Cache hit for '%s'\n", filepath.Base(result.Downloadable.URL))
			if err := scoop.extract(app, resolvedApp, cacheDir, versionDir, *result.Downloadable, arch); err != nil {
				return fmt.Errorf("error extracting file '%s': %w", filepath.Base(result.Downloadable.URL), err)
			}
		case *FinishedDownload:
			fmt.Printf("Downloaded '%s'\n", filepath.Base(result.Downloadable.URL))
			if err := scoop.extract(app, resolvedApp, cacheDir, versionDir, *result.Downloadable, arch); err != nil {
				return fmt.Errorf("error extracting file '%s': %w", filepath.Base(result.Downloadable.URL), err)
			}
		}
	}

	if installer := resolvedApp.Installer; installer != nil {
		dir := filepath.Join(scoop.AppDir(), app.Name, app.Version)
		if err := installer.invoke(scoop, dir, arch); err != nil {
			return fmt.Errorf("error invoking installer: %w", err)
		}
	}

	// FIXME Make copy util?
	// FIXME Read perms?
	newManifestFile, err := os.OpenFile(
		filepath.Join(versionDir, "manifest.json"), os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		return fmt.Errorf("error creating new manifest: %w", err)
	}
	if _, err := io.Copy(newManifestFile, manifestFile); err != nil {
		return fmt.Errorf("error copying manfiest: %w", err)
	}

	fmt.Println("Linking to newly installed version.")
	currentDir := filepath.Join(appDir, "current")
	if err := windows.CreateJunctions([2]string{versionDir, currentDir}); err != nil {
		return fmt.Errorf("error linking from new current dir: %w", err)
	}

	// Shims are copies of a certain binary that uses a ".shim" file next to
	// it to realise some type of symlink.
	for _, bin := range resolvedApp.Bin {
		fmt.Printf("Creating shim for '%s'\n", bin.Name)
		if err := scoop.CreateShim(filepath.Join(currentDir, bin.Name), bin); err != nil {
			return fmt.Errorf("error creating shim: %w", err)
		}
	}

	var envVars [][2]string
	if len(resolvedApp.EnvAddPath) > 0 {
		pathKey, oldPath, err := windows.GetPersistentEnvValue("Path")
		if err != nil {
			return fmt.Errorf("error attempt to add variables to path: %w", err)
		}
		parsedPath := windows.ParsePath(oldPath).Prepend(resolvedApp.EnvAddPath...)
		envVars = append(envVars, [2]string{pathKey, parsedPath.String()})
	}

	persistDir := filepath.Join(scoop.PersistDir(), app.Name)
	for _, pathEntry := range resolvedApp.EnvSet {
		value := substituteVariables(pathEntry.Value, map[string]string{
			"dir":         currentDir,
			"persist_dir": persistDir,
		})
		envVars = append(envVars, [2]string{pathEntry.Key, value})
	}

	if err := windows.SetPersistentEnvValues(envVars...); err != nil {
		return fmt.Errorf("error setting env values: %w", err)
	}

	// FIXME Adjust arch value if we install anything else than is desired.
	if err := os.WriteFile(filepath.Join(versionDir, "install.json"), []byte(fmt.Sprintf(
		`{
    "bucket": "%s",
    "architecture": "%s",
    "hold": %v
}`, app.Bucket.Name(), arch, version != "")), 0o600); err != nil {
		return fmt.Errorf("error writing installation information: %w", err)
	}

	if len(resolvedApp.Shortcuts) > 0 {
		startmenuPath, err := scoop.ShortcutDir()
		if err != nil {
			return err
		}

		var winShortcuts []windows.Shortcut
		for _, shortcut := range resolvedApp.Shortcuts {
			var winShortcut windows.Shortcut
			winShortcut.Dir = filepath.Join(startmenuPath, filepath.Dir(shortcut.ShortcutName))
			winShortcut.LinkTarget = filepath.Join(currentDir, shortcut.Name)
			winShortcut.Alias = filepath.Base(shortcut.ShortcutName)
			if shortcut.Icon != "" {
				winShortcut.Icon = filepath.Join(currentDir, shortcut.Icon)
			}
			winShortcut.Args = substituteVariables(shortcut.Args, map[string]string{
				"dir":          currentDir,
				"original_dir": versionDir,
				"persist_dir":  persistDir,
			})
			winShortcuts = append(winShortcuts, winShortcut)
		}

		if err := windows.CreateShortcuts(winShortcuts...); err != nil {
			return fmt.Errorf("error creating shortcuts: %w", err)
		}
	}

	for _, entry := range resolvedApp.Persist {
		// While I did find one manifest with $dir in it, said manifest installs
		// in a faulty way. (See versions/lynx283). The manifest hasn't really
		// been touched for at least 5 years. Either this was a scoop feature at
		// some point or it never was and went uncaught.

		source := filepath.Join(versionDir, entry.Dir)
		var target string
		if entry.LinkName != "" {
			target = filepath.Join(persistDir, entry.LinkName)
		} else {
			target = filepath.Join(persistDir, entry.Dir)
		}

		_, targetErr := os.Stat(target)
		if targetErr != nil && !os.IsNotExist(targetErr) {
			return targetErr
		}
		_, sourceErr := os.Stat(source)
		if sourceErr != nil && !os.IsNotExist(sourceErr) {
			return sourceErr
		}

		// Target exists
		if targetErr == nil {
			if sourceErr == nil {
				// "Backup" the source. Scoop did this as well.
				if err := os.Rename(source, source+".original"); err != nil {
					return fmt.Errorf("error backing up source: %w", err)
				}
			}
		} else if sourceErr == nil {
			if err := os.Rename(source, target); err != nil {
				return fmt.Errorf("error moving source to target: %w", err)
			}
		} else {
			if err := os.MkdirAll(target, os.ModeDir); err != nil {
				return fmt.Errorf("error creating target: %w", err)
			}
		}

		targetInfo, err := os.Stat(target)
		if err != nil {
			return err
		}

		if targetInfo.IsDir() {
			err = windows.CreateJunctions([2]string{target, source})
		} else {
			err = os.Link(target, source)
		}
		if err != nil {
			return fmt.Errorf("error linking to persist target: %w", err)
		}
	}
	if err := scoop.runScript(resolvedApp.PostInstall); err != nil {
		return fmt.Errorf("error running post install script: %w", err)
	}

	return nil
}

func substituteVariables(value string, variables map[string]string) string {
	// It seems like scoop does it this way as well. Instead of somehow checking
	// whether there's a variable such as $directory, we simply replace $dir,
	// not paying attention to potential damage done.
	// FIXME However, this is error prone and should change in the future.
	for key, val := range variables {
		value = strings.ReplaceAll(value, key, val)
	}

	// FIXME Additionally, we need to substitute any $env:VARIABLE. The bullet
	// proof way to do this, would be to simply invoke powershell, albeit a bit
	// slow. This should happen before the in-code substitution.

	// This needs more investigation though, should probably read the docs on
	// powershell env var substitution and see how easy it would be.

	return value
}

// extract will extract the given item. It doesn't matter which type it has, as
// this function will call the correct function. For example, a `.msi` will
// cause invocation of `lessmesi`. Note however, that this function isn't
// thread-safe, as it might install additional tooling required for extraction.
func (scoop *Scoop) extract(
	app *App,
	resolvedApp *AppResolved,
	cacheDir string,
	appDir string,
	item Downloadable,
	arch ArchitectureKey,
) error {
	baseName := filepath.Base(item.URL)
	fmt.Printf("Extracting '%s' ...\n", baseName)

	fileToExtract := filepath.Join(cacheDir, CachePath(app.Name, app.Version, item.URL))
	destinationDir := filepath.Join(appDir, item.ExtractTo)

	// Depending on metadata / filename, we decide how to extract the
	// files that are to be installed. Note we don't care whether the
	// dependency is installed via scoop, we just want it to be there.

	// We won't even bother testing the extension here, as it could
	// technically be an installed not ending on `.exe`. While this is
	// not true for the other formats, it is TECHNCIALLY possible here.
	if resolvedApp.InnoSetup {
		// If this flag is set, the installer.script might be set, but the
		// installer.file never is, meaning extraction is always the correct
		// thing to do.

		innounpPath, err := exec.LookPath("innounp")
		if err == nil && innounpPath != "" {
			goto INVOKE_INNOUNP
		}

		if err != nil {
			return fmt.Errorf("error looking up innounp: %w", err)
		}

		if err := scoop.Install("innounp", arch); err != nil {
			return fmt.Errorf("error installing dependency innounp: %w", err)
		}

	INVOKE_INNOUNP:
		args := []string{
			// Extract
			"-x",
			// Confirm questions
			"-y",
			// Destination
			"-d" + destinationDir,
			fileToExtract,
		}

		if strings.HasPrefix(item.ExtractDir, "{") {
			args = append(args, "-c"+item.ExtractDir)
		} else if item.ExtractDir != "" {
			args = append(args, "-c{app}\\"+item.ExtractDir)
		} else {
			args = append(args, "-c{app}")
		}

		cmd := exec.Command("innounp", args...)
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("error invoking innounp: %w", err)
		}

		return nil
	}

	ext := strings.ToLower(filepath.Ext(item.URL))
	// 7zip supports A TON of file formats, so we try to use it where we
	// can. It's fast and known to work well.
	if supportedBy7Zip(ext) {
		sevenZipPath, err := exec.LookPath("7z")
		// Path can be non-empty and still return an error. Read
		// LookPath documentation.
		if err == nil && sevenZipPath != "" {
			goto INVOKE_7Z
		}

		// Fallback for cases where we don't have 7zip installed, but still
		// want to unpack a zip. Without this, we'd print an error instead.
		if ext == ".zip" {
			goto STD_ZIP
		}

		if err != nil {
			return fmt.Errorf("error doing path lookup: %w", err)
		}

		if err := scoop.Install("7zip", arch); err != nil {
			return fmt.Errorf("error installing dependency 7zip: %w", err)
		}

	INVOKE_7Z:
		args := []string{
			// Extract from file
			"x",
			fileToExtract,
			// Target path
			"-o" + destinationDir,
			// Overwrite all files
			"-aoa",
			// Confirm
			"-y",
		}

		// FIXME: $IsTar = ((strip_ext $Path) -match '\.tar$') -or ($Path -match '\.t[abgpx]z2?$')
		if ext != ".tar" && item.ExtractDir != "" {
			args = append(args, "-ir!"+item.ExtractDir+"\\*")
		}
		cmd := exec.Command(
			"7z",
			args...,
		)
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("error invoking 7z: %w", err)
		}

		// 7zip can't extract the ExtractDir files, it always creates the
		// extract dir.
		if item.ExtractDir != "" {
			dirToMove := filepath.Join(destinationDir, item.ExtractDir)
			if err := windows.ExtractDir(dirToMove, destinationDir); err != nil {
				return fmt.Errorf("error extracing dir: %w", err)
			}
			// Since the dir should be empty by now, a simple remove will delete
			// it, no need for RemoveAll.
			if err := os.Remove(dirToMove); err != nil {
				return fmt.Errorf("error cleaning up empty directory: %w", err)
			}
		}

		return nil
	}

	// TODO: dark, msi, installer, zst

	switch ext {
	case ".msi":
		lessmsiPath, err := scoop.ensureExecutable("lessmsi", "lessmsi", arch)
		if err != nil {
			return fmt.Errorf("error installing lessmsi: %w", err)
		}
		fmt.Println(lessmsiPath)

		return nil
	}

STD_ZIP:
	if ext == ".zip" {
		zipReader, err := zip.OpenReader(fileToExtract)
		if err != nil {
			return fmt.Errorf("error opening zip reader: %w", err)
		}

		for _, f := range zipReader.File {
			// We create these anyway later.
			if f.FileInfo().IsDir() {
				continue
			}

			// FIXME Prevent accidental mismatches
			extractDir := filepath.ToSlash(item.ExtractDir)
			fName := filepath.ToSlash(f.Name)
			if extractDir != "" && !strings.HasPrefix(fName, extractDir) {
				continue
			}

			// Strip extract dir, as these aren't meant to be preserved,
			// unless specified via extractTo
			fName = strings.TrimLeft(strings.TrimPrefix(fName, extractDir), "/")

			filePath := filepath.Join(appDir, item.ExtractTo, fName)
			if err := os.MkdirAll(filepath.Dir(filePath), os.ModePerm); err != nil {
				return fmt.Errorf("error creating dir: %w", err)
			}

			dstFile, err := os.OpenFile(filePath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, f.Mode())
			if err != nil {
				return fmt.Errorf("error creating target file for zip entry: %w", err)
			}

			fileInArchive, err := f.Open()
			if err != nil {
				return fmt.Errorf("error opening zip file entry: %w", err)
			}

			if _, err := io.Copy(dstFile, fileInArchive); err != nil {
				return fmt.Errorf("error copying zip file entry: %w", err)
			}

			dstFile.Close()
			fileInArchive.Close()
		}
	} else {
		targetFile, err := os.OpenFile(
			filepath.Join(appDir, item.ExtractTo, baseName),
			os.O_CREATE|os.O_WRONLY|os.O_TRUNC,
			0o600,
		)
		if err != nil {
			return fmt.Errorf("error opening handle target file: %w", err)
		}
		defer targetFile.Close()

		sourceFile, err := os.Open(fileToExtract)
		if err != nil {
			return fmt.Errorf("error opening cache file: %w", err)
		}
		defer sourceFile.Close()

		if _, err := io.Copy(targetFile, sourceFile); err != nil {
			return fmt.Errorf("error copying file: %w", err)
		}
	}

	// Mark RO afterwards?
	return nil
}

// ensureExecutable will look for a given executable on the path. If not
// found, it will attempt installing the dependency using the given app
// information.
func (scoop *Scoop) ensureExecutable(executable, appName string, arch ArchitectureKey) (string, error) {
	executablePath, err := exec.LookPath(executable)
	if err != nil {
		if !errors.Is(err, exec.ErrDot) && !errors.Is(err, exec.ErrNotFound) {
			return "", fmt.Errorf("error locating '%s': %w", executable, err)
		}

		// We'll treat a relative path binary as non-existent for now and
		// install the dependency.
		executablePath = ""
	}

	if executablePath == "" {
		if err := scoop.Install(appName, arch); err != nil {
			return "", fmt.Errorf("error installing required dependency '%s': %w", appName, err)
		}

		executablePath, err = exec.LookPath(executable)
		if err != nil {
			return "", fmt.Errorf("error locating '%s': %w", executable, err)
		}
	}

	// Might be empty if the second lookup failed. HOWEVER, it shouldn't as we
	// simply add to the shims folder, which should already be on the path.
	return executablePath, err
}

var sevenZipFileFormatRegex = regexp.MustCompile(`\.((gz)|(tar)|(t[abgpx]z2?)|(lzma)|(bz2?)|(7z)|(001)|(rar)|(iso)|(xz)|(lzh)|(nupkg))(\.[^\d.]+)?$`)

func supportedBy7Zip(extension string) bool {
	return sevenZipFileFormatRegex.MatchString(extension)
}

// AppResolved is a version of app forming the data into a way that it's ready
// for installation, deinstallation or update.
type AppResolved struct {
	*App

	// TODO checkver

	Bin       []Bin      `json:"bin"`
	Shortcuts []Shortcut `json:"shortcuts"`

	Downloadables []Downloadable `json:"downloadables"`

	// Installer deprecates msi; InnoSetup bool should be same for each
	// architecture. The docs don't mention it.
	Installer   *Installer `json:"installer"`
	PreInstall  []string   `json:"pre_install"`
	PostInstall []string   `json:"post_install"`
}

// ForArch will create a merged version that includes all the relevant fields at
// root level. Access to architecture shouldn't be required anymore, it should
// be ready to use for installtion, update or uninstall.
func (a *App) ForArch(arch ArchitectureKey) *AppResolved {
	resolved := &AppResolved{
		App: a,
	}

	resolved.Bin = a.Bin
	resolved.Shortcuts = a.Shortcuts
	resolved.Downloadables = a.Downloadables
	resolved.PreInstall = a.PreInstall
	resolved.PostInstall = a.PostInstall
	resolved.Installer = a.Installer

	if a.Architecture == nil {
		return resolved
	}

	archValue := a.Architecture[arch]
	if archValue == nil && arch == ArchitectureKey64Bit {
		// Fallbackt to 32bit. If we are on arm, there's no use to fallback
		// though, since only arm64 is supported by scoop either way.
		archValue = a.Architecture[ArchitectureKey32Bit]
	}
	if archValue != nil {
		// nil-checking might be fragile, so this is safer.
		if len(archValue.Bin) > len(resolved.Bin) {
			resolved.Bin = archValue.Bin
		}
		if len(archValue.Shortcuts) > len(resolved.Shortcuts) {
			resolved.Shortcuts = archValue.Shortcuts
		}
		if len(archValue.Downloadables) > len(resolved.Downloadables) {
			// If we need to manipulate these, we do a copy, to prevent changing the
			// opriginal app.
			if len(a.ExtractTo) > 0 {
				resolved.Downloadables = append([]Downloadable{}, archValue.Downloadables...)
			} else {
				resolved.Downloadables = archValue.Downloadables
			}
		}
		if len(archValue.PreInstall) > len(resolved.PreInstall) {
			resolved.PreInstall = archValue.PreInstall
		}
		if len(archValue.PostInstall) > len(resolved.PostInstall) {
			resolved.PostInstall = archValue.PostInstall
		}
	}

	// architecture does not support extract_to, so we merge it with the root
	// level value for ease of use.
	switch len(a.ExtractTo) {
	case 0:
		// Do nothing, path inferred to app root dir (current).
	case 1:
		// Same path everywhere
		for i := 0; i < len(resolved.Downloadables); i++ {
			resolved.Downloadables[i].ExtractTo = a.ExtractTo[0]
		}
	default:
		// Path per URL, but to be defensive, we'll infer if missing ones, by
		// leaving it empty (current root dir).
		for i := 0; i < len(resolved.Downloadables) && i < len(a.ExtractTo); i++ {
			resolved.Downloadables[i].ExtractTo = a.ExtractTo[i]
		}
	}

	// If we have neither an installer file, nor a script, we reference the last
	// items downloaded, as per scoop documentation.
	// FIXME Find out if this is really necessary, this is jank.
	if a.Installer != nil && a.Installer.File == "" &&
		len(a.Installer.Script) == 0 && len(a.Downloadables) > 0 {
		lastURL := resolved.Downloadables[len(a.Downloadables)-1].URL
		a.Installer.File = filepath.Base(lastURL)
	}

	return resolved
}

var ErrBucketNoGitDir = errors.New(".git dir at path not found")

func (a *App) AvailableVersions() ([]string, error) {
	return a.AvailableVersionsN(math.MaxInt32)
}

func (a *App) AvailableVersionsN(maxVersions int) ([]string, error) {
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
		if len(versions) >= maxVersions {
			break
		}
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
	json.ResetBytes(iter, data)

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
func (a *App) ManifestForVersion(targetVersion string) (io.ReadSeeker, error) {
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
			return bytes.NewReader(result.Data), nil
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

	return filepath.Glob(filepath.Join(scoop.CacheDir(), expectedPrefix+"*"))
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

func (scoop *Scoop) CacheDir() string {
	return filepath.Join(scoop.scoopRoot, "cache")
}

type Scoop struct {
	scoopRoot string
}

func (scoop *Scoop) AppDir() string {
	return filepath.Join(scoop.scoopRoot, "apps")
}

func (scoop Scoop) ShortcutDir() (string, error) {
	startmenuPath, err := windows.GetFolderPath("StartMenu")
	if err != nil {
		return "", fmt.Errorf("error determining start menu path: %w", err)
	}
	return filepath.Join(startmenuPath, "Programs", "Scoop Apps"), nil
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
