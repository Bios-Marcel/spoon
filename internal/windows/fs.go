package windows

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

// ExtractDir will move all files of a given to the target directory
func ExtractDir(dirToExtract, destinationDir string) error {
	return filepath.WalkDir(
		dirToExtract,
		func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}

			// We don't care about the root, which is what we  are trying
			// to remove.
			if path == dirToExtract {
				return nil
			}

			// We take the relative path to be able to drop the first
			// element, which is the extractDir.
			relPath, err := filepath.Rel(dirToExtract, path)
			if err != nil {
				return err
			}

			if err := os.Rename(path, filepath.Join(destinationDir, relPath)); err != nil {
				return err
			}

			if d.IsDir() {
				return filepath.SkipDir
			}

			return nil
		})
}

// ForceRemoveAll will delete any file or folder recursively. This will not
// delete the content of junctions.
func ForceRemoveAll(path string) error {
	if err := os.Remove(path); err == nil || os.IsNotExist(err) {
		return nil
	}

	if err := os.Chmod(path, 0o600); err != nil {
		return fmt.Errorf("error marking path read-only")
	}

	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("error stating file for deletion: %w", err)
	}

	if !info.IsDir() {
		// Try to delete again, now that it is marked read-only
		return os.Remove(path)
	}

	// This also resolves junctions.
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		return fmt.Errorf("error resolving links: %w", err)
	}

	// Workaround for junctions, as they can be deleted directly. Can we use the
	// stat call to identify the junction?
	if resolved != path {
		if err := os.Remove(path); err == nil || os.IsNotExist(err) {
			return nil
		}
	}

	files, err := GetDirFilenames(path)
	if err != nil {
		return fmt.Errorf("error reading dir names: %w", err)
	}

	for _, file := range files {
		if err := ForceRemoveAll(filepath.Join(path, file)); err != nil {
			return err
		}
	}

	// Remove empty dir that's leftover.
	return os.Remove(path)
}

// ForceRemoveAllBestEffort deletes path recursively without stopping after the
// first failure. Junctions and symbolic links are removed without traversing
// them. The returned error contains every path that could not be removed.
func ForceRemoveAllBestEffort(path string) error {
	var errs []error
	forceRemoveAllBestEffort(path, &errs)
	return errors.Join(errs...)
}

func forceRemoveAllBestEffort(path string, errs *[]error) {
	// This handles files, empty directories and links in the common case.
	if err := os.Remove(path); err == nil || os.IsNotExist(err) {
		return
	}

	info, err := os.Lstat(path)
	if err != nil {
		if !os.IsNotExist(err) {
			*errs = append(*errs, fmt.Errorf("error stating %q for deletion: %w", path, err))
		}
		return
	}

	link, err := IsLink(path)
	if err != nil {
		*errs = append(*errs, fmt.Errorf("error checking whether %q is a link: %w", path, err))
		return
	}

	mode := fs.FileMode(0o600)
	if info.IsDir() && !link {
		mode = 0o700
	}
	// On Windows this also clears the read-only attribute that Scoop sets on
	// junctions. A chmod failure is only relevant if deletion still fails.
	chmodErr := os.Chmod(path, mode)

	if link || !info.IsDir() {
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			if chmodErr != nil {
				*errs = append(*errs, fmt.Errorf("error making %q writable: %w", path, chmodErr))
			}
			*errs = append(*errs, fmt.Errorf("error removing %q: %w", path, err))
		}
		return
	}

	entries, err := os.ReadDir(path)
	if err != nil {
		*errs = append(*errs, fmt.Errorf("error reading %q for deletion: %w", path, err))
	} else {
		for _, entry := range entries {
			forceRemoveAllBestEffort(filepath.Join(path, entry.Name()), errs)
		}
	}

	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		if chmodErr != nil {
			*errs = append(*errs, fmt.Errorf("error making %q writable: %w", path, chmodErr))
		}
		*errs = append(*errs, fmt.Errorf("error removing %q: %w", path, err))
	}
}

func GetDirFilenames(dir string) ([]string, error) {
	dirHandle, err := os.Open(dir)
	if err != nil {
		return nil, err
	}
	defer dirHandle.Close()
	return dirHandle.Readdirnames(-1)
}

// CreateJunction will create multiple junctions. Each pair reflects one
// junction ([2]string{existing_dir, link_location}). A junction as a hardlink
// to a directory. This means, that we hardlink to the drive-sections, which
// results in deletions not affecting the actual data, as long as there are
// still references.
func CreateJunctions(junctions ...[2]string) error {
	for _, junction := range junctions {
		from, err := filepath.Abs(junction[0])
		if err != nil {
			return fmt.Errorf("error creating absolute path: %w", err)
		}
		junction[0] = from

		to, err := filepath.Abs(junction[1])
		if err != nil {
			return fmt.Errorf("error creating absolute path: %w", err)
		}
		junction[1] = to
	}

	scriptLines := make([]string, 0, len(junctions))
	for _, junction := range junctions {
		// No need to re-create a junction
		if _, err := os.Stat(junction[1]); err == nil {
			continue
		}

		scriptLines = append(scriptLines, fmt.Sprintf(`mklink /J "%s" "%s"`, junction[1], junction[0]))
	}

	return RunAndPipeInto("cmd", nil, scriptLines)
}
