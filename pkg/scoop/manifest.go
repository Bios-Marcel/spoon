package scoop

import (
	"fmt"
	"io"
	"os"
	"slices"
	"strings"

	jsoniter "github.com/json-iterator/go"
)

const (
	DetailFieldBin           = "bin"
	DetailFieldShortcuts     = "shortcuts"
	DetailFieldUrl           = "url"
	DetailFieldHash          = "hash"
	DetailFieldArchitecture  = "architecture"
	DetailFieldDescription   = "description"
	DetailFieldVersion       = "version"
	DetailFieldNotes         = "notes"
	DetailFieldDepends       = "depends"
	DetailFieldEnvSet        = "env_set"
	DetailFieldEnvAddPath    = "env_add_path"
	DetailFieldExtractDir    = "extract_dir"
	DetailFieldExtractTo     = "extract_to"
	DetailFieldPostInstall   = "post_install"
	DetailFieldPreInstall    = "pre_install"
	DetailFieldPreUninstall  = "pre_uninstall"
	DetailFieldPostUninstall = "post_uninstall"
	DetailFieldInstaller     = "installer"
	DetailFieldUninstaller   = "uninstaller"
	DetailFieldInnoSetup     = "innosetup"
)

// DetailFieldsAll is a list of all available DetailFields to load during
// [App.LoadDetails]. Use these if you need all fields or don't care whether
// unneeded fields are being loaded.
var DetailFieldsAll = []string{
	DetailFieldBin,
	DetailFieldShortcuts,
	DetailFieldUrl,
	DetailFieldHash,
	DetailFieldArchitecture,
	DetailFieldDescription,
	DetailFieldVersion,
	DetailFieldNotes,
	DetailFieldDepends,
	DetailFieldEnvSet,
	DetailFieldEnvAddPath,
	DetailFieldExtractDir,
	DetailFieldExtractTo,
	DetailFieldPostInstall,
	DetailFieldPreInstall,
	DetailFieldPreUninstall,
	DetailFieldPostUninstall,
	DetailFieldInstaller,
	DetailFieldUninstaller,
	DetailFieldInnoSetup,
}

// manifestIter gives you an iterator with a big enough size to read any
// manifest without reallocations.
func manifestIter() *jsoniter.Iterator {
	return jsoniter.Parse(jsoniter.ConfigFastest, nil, 1024*128)
}

// LoadDetails will load additional data regarding the manifest, such as
// description and version information. This causes IO on your drive and
// therefore isn't done by default.
func (a *App) LoadDetails(fields ...string) error {
	return a.LoadDetailsWithIter(manifestIter(), fields...)
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

	return a.loadDetailFromManifestWithIter(iter, file, fields...)
}

func mergeIntoDownloadables(urls, hashes, extractDirs, extractTos []string) []Downloadable {
	// It can happen that we have different extract_dirs, but only one archive,
	// containing both architectures. This should also never be empty, but at
	// least of size one, so we'll never allocate for naught.
	downloadables := make([]Downloadable, max(len(urls), len(extractDirs), len(extractTos)))

	// We assume that we have the same length in each. While this
	// hasn't been specified in the app manifests wiki page, it's
	// the seemingly only sensible thing to me.
	// If we are missing extract_dir or extract_to entries, it's fine, as we use
	// nonpointer values anyway and simple default to empty, which means
	// application directory.
	for index, value := range urls {
		downloadables[index].URL = value
	}
	for index, value := range hashes {
		downloadables[index].Hash = value
	}
	for index, value := range extractDirs {
		downloadables[index].ExtractDir = value
	}
	for index, value := range extractTos {
		downloadables[index].ExtractTo = value
	}

	return downloadables
}

// LoadDetails will load additional data regarding the manifest, such as
// description and version information. This causes IO on your drive and
// therefore isn't done by default.
func (a *App) loadDetailFromManifestWithIter(
	iter *jsoniter.Iterator,
	manifest io.Reader,
	fields ...string,
) error {
	iter.Reset(manifest)

	var urls, hashes, extractDirs, extractTos []string
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
			urls = parseStringOrArray(iter)
		case DetailFieldHash:
			hashes = parseStringOrArray(iter)
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
					case "installer":
						installer := parseInstaller(iter)
						archValue.Installer = &installer
					case "uninstaller":
						uninstaller := Uninstaller(parseInstaller(iter))
						archValue.Uninstaller = &uninstaller
					default:
						iter.Skip()
					}
				}

				// extract_to is always on the root level, so we pass nil
				archValue.Downloadables = mergeIntoDownloadables(urls, hashes, extractDirs, nil)
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
			installer := parseInstaller(iter)
			a.Installer = &installer
		case DetailFieldUninstaller:
			uninstaller := Uninstaller(parseInstaller(iter))
			a.Uninstaller = &uninstaller
		case DetailFieldInnoSetup:
			a.InnoSetup = iter.ReadBool()
		case DetailFieldPreInstall:
			a.PreInstall = parseStringOrArray(iter)
		case DetailFieldPostInstall:
			a.PostInstall = parseStringOrArray(iter)
		case DetailFieldPreUninstall:
			a.PreUninstall = parseStringOrArray(iter)
		case DetailFieldPostUninstall:
			a.PostUninstall = parseStringOrArray(iter)
		case DetailFieldExtractDir:
			extractDirs = parseStringOrArray(iter)
		case DetailFieldExtractTo:
			extractTos = parseStringOrArray(iter)
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

	// If there are no URLs at the root level, that means they are in the
	// arch-specific instructions. In this case, we'll only access the
	// ExtractTo / ExtractDir when resolving a certain arch.
	if len(urls) > 0 {
		a.Downloadables = mergeIntoDownloadables(urls, hashes, extractDirs, extractTos)
	}

	return nil
}

func parseInstaller(iter *jsoniter.Iterator) Installer {
	installer := Installer{}
	for field := iter.ReadObject(); field != ""; field = iter.ReadObject() {
		switch field {
		case "file":
			installer.File = iter.ReadString()
		case "script":
			installer.Script = parseStringOrArray(iter)
		case "args":
			installer.Args = parseStringOrArray(iter)
		case "keep":
			installer.Keep = iter.ReadBool()
		default:
			iter.Skip()
		}
	}
	return installer
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
