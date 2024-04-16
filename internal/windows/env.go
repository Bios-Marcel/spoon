package windows

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"slices"
	"strings"
)

type Paths []string

// ParsePath will break the path variable content down into separate paths. This
// also handles quoting. The order is preserved.
func ParsePath(value string) Paths {
	// Technically we could also use strings.FieldFunc, but we got to manually
	// cut of the quotes anyway then, so we'll just do the whole thing manually.
	var values []string
	var quoteOpen bool
	var nextStart int
	for index, char := range value {
		if char == '"' {
			if quoteOpen {
				// +1 to skip the open quote
				values = append(values, value[nextStart+1:index])
				// End quote means we'll have a separator next, so we start at
				// the next path char.
				nextStart = index + 2
			}

			quoteOpen = !quoteOpen
		} else if char == ';' && index > nextStart {
			if quoteOpen {
				continue
			}

			values = append(values, value[nextStart:index])
			nextStart = index + 1
		}
	}

	// Last element if applicable, since the path could also end on a semicolon
	// or quote.
	if nextStart < len(value) {
		values = append(values, value[nextStart:])
	}

	return Paths(values)
}

// Remove returns a new path object that doesn't contain any of the specified
// paths.
func (p Paths) Remove(paths ...string) Paths {
	p = slices.DeleteFunc(p, func(value string) bool {
		// FIXME This should sanitize the path separators and such. We also need
		// tests for this.
		return slices.Contains(paths, value)
	})
	return p
}

// Preprend will create a new Paths object, adding the supplied paths infront,
// using the given order.
func (p Paths) Prepend(paths ...string) Paths {
	newPath := make(Paths, 0, len(p)+len(paths))
	newPath = append(newPath, paths...)
	newPath = append(newPath, p...)
	return newPath
}

// Creates a new path string, where all entries are quoted.
func (p Paths) String() string {
	var buffer bytes.Buffer
	for i := 0; i < len(p); i++ {
		if i != 0 {
			buffer.WriteRune(';')
		}

		// FIXME Only quote if necessary? Only if contains semicolon?
		buffer.WriteRune('"')
		buffer.WriteString(p[i])
		buffer.WriteRune('"')
	}
	return buffer.String()
}

func GetPersistentEnvValues() (map[string]string, error) {
	cmd := exec.Command(
		"powershell",
		"-NoLogo",
		"-NoProfile",
		"[Environment]::GetEnvironmentVariables('User') | ConvertTo-Json",
	)
	pipe, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("error opening pipe: %w", err)
	}

	var cmdErr error
	go func() {
		cmdErr = cmd.Run()
	}()

	decoder := json.NewDecoder(pipe)
	result := make(map[string]string)
	if err := decoder.Decode(&result); err != nil {
		return nil, fmt.Errorf("error decoding environment variables: %w", err)
	}

	if cmdErr != nil {
		return nil, fmt.Errorf("error retrieving environment variables: %w", err)
	}

	return result, nil
}

// GetPersistentEnvValue retrieves a persistent user level environment variable.
// The first returned value is the key and the second the value. While the key
// is defined in the query, the casing might be different, which COULD matter.
// If nothing was found, we return empty strings without an error.
func GetPersistentEnvValue(key string) (string, string, error) {
	// While we could directly call GetEnvironmentVariable, we want to find out
	// he string, therefore we use the result of the GetAll call.

	allVars, err := GetPersistentEnvValues()
	if err != nil {
		return "", "", fmt.Errorf("error retrieving variables: %w", err)
	}

	for keyPersisted, val := range allVars {
		if strings.EqualFold(key, keyPersisted) {
			return keyPersisted, val, nil
		}
	}
	return "", "", nil
}

// Sets a User-Level Environment variable. An empty value will remove the key
// completly.
func SetPersistentEnvValue(key, value string) error {
	cmd := exec.Command(
		"powershell",
		"-NoProfile",
		"-NoLogo",
		"-Command",
		"[Environment]::SetEnvironmentVariable('"+key+"','"+value+"','User')",
	)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin
	return cmd.Run()
}

func SetPersistentEnvValues(vars ...[2]string) error {
	if len(vars) == 0 {
		return nil
	}

	var command bytes.Buffer
	for _, pair := range vars {
		command.WriteString("[Environment]::SetEnvironmentVariable('")
		command.WriteString(pair[0])
		command.WriteString("','")
		command.WriteString(pair[1])
		command.WriteString("','User');")
	}

	cmd := exec.Command(
		"powershell",
		"-NoProfile",
		"-NoLogo",
		"-Command",
		command.String(),
	)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin
	return cmd.Run()
}

// GetFolderPath is equivalent to [Environment]::GetFolderPath and determines
// user-specific folder locations.
func GetFolderPath(folderType string) (string, error) {
	cmd := exec.Command(
		"powershell",
		"-NoLogo",
		"-NoProfile",
		`[Environment]::GetFolderPath("`+folderType+`")`,
	)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", err
	}

	// Trim, since we don't want the newline.
	return strings.TrimSpace(string(output)), nil
}
