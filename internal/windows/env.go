package windows

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
)

func GetPersistentEnvValues() (map[string]string, error) {
	cmd := exec.Command(
		"powershell",
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

// Sets a User-Level Environment variable. An empty value will remove the key
// completly.
func SetPersistentEnvValue(key, value string) error {
	cmd := exec.Command(
		"powershell",
		"-NoProfile",
		"-Command",
		"[Environment]::SetEnvironmentVariable('"+key+"','"+value+"','User')",
	)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin
	return cmd.Run()
}
