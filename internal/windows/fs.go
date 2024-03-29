package windows

import (
	"fmt"
	"os"
	"os/exec"
)

func GetDirFilenames(dir string) ([]string, error) {
	dirHandle, err := os.Open(dir)
	if err != nil {
		return nil, err
	}
	return dirHandle.Readdirnames(-1)
}

// CreateJunction will create multiple junctions. Each pair reflects one
// junction ([2]string{existing_dir, link_location}). A junction as a hardlink
// to a directory. This means, that we hardlink to the drive-sections, which
// results in deletions not affecting the actual data, as long as there are
// still references.
func CreateJunctions(junctions ...[2]string) error {
	for _, junction := range junctions {
		from := junction[0]
		to := junction[1]
		// No need to re-create a junction
		if _, err := os.Stat(to); err == nil {
			return nil
		}

		cmd := exec.Command("cmd", "/c", "mklink", "/J", to, from)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		cmd.Stdin = os.Stdin

		if err := cmd.Run(); err != nil {
			return fmt.Errorf("error creating junction to '%s': %w", to, err)
		}
	}

	return nil
}
