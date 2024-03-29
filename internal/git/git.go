package git

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

func FindRepoRoot(path string) string {
	repoPath := path
	for {
		if _, err := os.Stat(filepath.Join(repoPath, ".git")); err == nil {
			// The dir we've reached is the root of the bucket and therefore a
			// git repository.
			return repoPath
		}

		// We've reached the root of the volume.
		if newPath := filepath.Dir(repoPath); newPath != repoPath {
			repoPath = newPath
			continue
		}

		// We've reached the root of the volume.
		return ""
	}
}

func backgroundCommand(
	ctx context.Context,
	errors chan error,
	workingDirectory string,
	executable string,
	args ...string,
) (io.ReadCloser, error) {
	cmd := exec.CommandContext(ctx, executable, args...)
	cmd.Dir = workingDirectory
	out, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("error opening stdout pipe: %w", err)
	}

	go func() {
		// If we error, we close the pipe to indirectly notify callers of the
		// failure.
		defer out.Close()
		if err := cmd.Run(); err != nil && errors != nil {
			errors <- err
		}
	}()

	return out, nil
}

// GitPaths returns the repository root and the relative filepath of the given
// file. The paths use forward slashes, as this helps working with git.
func GitPaths(path string) (string, string) {
	path = filepath.ToSlash(path)
	repoPath := FindRepoRoot(path)
	if repoPath == "" {
		return "", ""
	}

	repoPath = filepath.ToSlash(repoPath)
	path = strings.TrimPrefix(path, repoPath)
	path = strings.TrimPrefix(path, "/")
	return repoPath, path
}

func FileCommits(
	ctx context.Context,
	repo, relFilePath string,
	errors chan error,
) (io.ReadCloser, error) {
	return backgroundCommand(ctx, errors, repo, "git", "log", `--pretty=format:%h`, "--", relFilePath)
}

func FileContent(
	ctx context.Context,
	repo, relFilePath, commit string,
	errors chan error,
) (io.ReadCloser, error) {
	fileReference := fmt.Sprintf("%s:%s", commit, relFilePath)
	return backgroundCommand(ctx, errors, repo, "git", "show", fileReference)
}

type FileContentResult struct {
	Data  []byte
	Error error
}

func FileContents(
	ctx context.Context,
	repo, relFilePath string,
	results chan FileContentResult,
) error {
	errors := make(chan error)
	outPipe, err := FileCommits(ctx, repo, relFilePath, errors)
	if err != nil {
		return fmt.Errorf("error retrieving file commits: %w", err)
	}

	go func() {
		defer close(results)
		scanner := bufio.NewScanner(outPipe)

		for scanner.Scan() {
			hash := scanner.Text()

			showOut, err := FileContent(ctx, repo, relFilePath, hash, errors)
			if err != nil {
				results <- FileContentResult{Error: err}
				return
			}
			defer showOut.Close()

			bytes, err := io.ReadAll(showOut)
			if err != nil {
				results <- FileContentResult{Error: err}
				return
			}

			results <- FileContentResult{Data: bytes}
			showOut.Close()
		}

		select {
		case err := <-errors:
			results <- FileContentResult{Error: err}
		default:
			return
		}
	}()

	return nil
}
