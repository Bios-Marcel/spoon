//go:build !windows

package windows

func Arch() string { return "amd64" }

type Shortcut struct {
	// Dir is the location to create the shortcut in
	Dir string
	// LinkTarget is the location of the executable to be run.
	LinkTarget string
	// Alias is the name displayed in the explorer / startmenu.
	Alias string
	// Args are optional commandline arguments.
	Args string
	// Icon is an optional image displayed in the explorer / startmenu.
	Icon string
}

func CreateShortcuts(shortcuts ...Shortcut) error {
	return nil
}

func GetShellExecutable() (string, error) {
	return "bash", nil
}

type Process struct {
	Pid        uint32
	Fullpath   string
	Executable string
}

func ProcessList() ([]Process, error) {
	return nil, nil
}

func ProcessKill(pid uint32) (bool, error) {
	return true, nil
}
