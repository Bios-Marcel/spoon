package scoop

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	_ "embed"
)

// //go:embed shim_jar_to_cmd.template
var jarToCmdTemplate string

// //go:embed shim_jar_to_bash.template
var jarToBashTemplate string

//go:embed shim_cmd_to_cmd.template
var cmdToCmdTemplate string

//go:embed shim_cmd_to_bash.template
var cmdToBashTemplate string

//go:embed shim.exe
var shimExecutable []byte

// FIXME Should this be a public helper function on Bin? If so, we should
// probably split bin and shortcut. At this point, they don't seem to be to
// compatible anymore.
func shimName(bin Bin) string {
	shimName := bin.Alias
	if shimName == "" {
		shimName = filepath.Base(bin.Name)
		shimName = strings.TrimSuffix(shimName, filepath.Ext(shimName))
	}
	return shimName
}

func (scoop *Scoop) RemoveShims(bins ...Bin) error {
	return filepath.WalkDir(scoop.ShimDir(), func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if d.IsDir() {
			return nil
		}

		for _, bin := range bins {
			// This will catch all file types, including the shims.
			shimName := shimName(bin)
			binWithoutExt := strings.TrimSuffix(shimName, filepath.Ext(shimName))
			nameWithoutExt := strings.TrimSuffix(d.Name(), filepath.Ext(d.Name()))
			if !strings.EqualFold(nameWithoutExt, binWithoutExt) {
				continue
			}

			if err := os.Remove(path); err != nil {
				return fmt.Errorf("error deleting shim '%s': %w", path, err)
			}
		}

		return nil
	})
}

func (scoop *Scoop) CreateShim(path string, bin Bin) error {
	/*
		We got the following possible constructs:

		0.
			bin: [
				path/to/file
			]
		1.
			bin: [
				[
				path/to/file
				shim.type
				]
			]
		2.
			bin: [
				[
				path/to/file.exe
				shim
				]
			]

		In case 0. we simply create whatever extension the file had as a
		shim, falling back to .cmd.

		In case 1. we create a shim given the desired extension, no matter
		what extension the actual file has. The same goes for case 2. where we
		haven't passed an explicit shim extension even though we know it's an
		executable.
	*/

	shimName := shimName(bin)

	switch filepath.Ext(bin.Name) {
	case ".exe", ".com":
		// FIXME Do we need to escape anything here?
		argsJoined := strings.Join(bin.Args, " ")

		// The .shim and .exe files needs to be writable, as scoop fails to
		// uninstall otherwise.
		var shimConfig bytes.Buffer
		shimConfig.WriteString(`path = "`)
		shimConfig.WriteString(path)
		shimConfig.WriteString("\"\n")
		if argsJoined != "" {
			shimConfig.WriteString(`args = `)
			shimConfig.WriteString(argsJoined)
			shimConfig.WriteRune('\n')
		}
		if err := os.WriteFile(filepath.Join(scoop.ShimDir(), shimName+".shim"),
			shimConfig.Bytes(), 0o600); err != nil {
			return fmt.Errorf("error writing shim file: %w", err)
		}

		targetPath := filepath.Join(scoop.ShimDir(), shimName+".exe")
		err := os.WriteFile(targetPath, shimExecutable, 0o700)
		if err != nil {
			return fmt.Errorf("error creating shim executable: %w", err)
		}
	case ".cmd", ".bat":
		// FIXME Do we need to escape anything here?
		argsJoined := strings.Join(bin.Args, " ")

		if err := os.WriteFile(
			filepath.Join(scoop.ShimDir(), shimName+".cmd"),
			[]byte(fmt.Sprintf(cmdToCmdTemplate, path, path, argsJoined)),
			0o700,
		); err != nil {
			return fmt.Errorf("error creating cmdShim: %w", err)
		}
		if err := os.WriteFile(
			filepath.Join(scoop.ShimDir(), shimName),
			[]byte(fmt.Sprintf(cmdToBashTemplate, path, path, argsJoined)),
			0o700,
		); err != nil {
			return fmt.Errorf("error creating cmdShim: %w", err)
		}
	case ".ps1":
	case ".jar":
		// FIXME Do we need to escape anything here?
		argsJoined := strings.Join(bin.Args, " ")

		if err := os.WriteFile(
			filepath.Join(scoop.ShimDir(), shimName+".cmd"),
			[]byte(fmt.Sprintf(jarToCmdTemplate, path, path, argsJoined)),
			0o700,
		); err != nil {
			return fmt.Errorf("error creating cmdShim: %w", err)
		}
		if err := os.WriteFile(
			filepath.Join(scoop.ShimDir(), shimName),
			[]byte(fmt.Sprintf(jarToBashTemplate, path, path, argsJoined)),
			0o700,
		); err != nil {
			return fmt.Errorf("error creating cmdShim: %w", err)
		}
	case ".py":
	default:
		// FIXME Do we want to implement this case?
		return errors.New("this package contains a currently unsupported shim-type, please contact the maintainer")
	}

	return nil
}

func (scoop *Scoop) ShimDir() string {
	return filepath.Join(scoop.scoopRoot, "shims")
}
