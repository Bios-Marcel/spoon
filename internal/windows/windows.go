package windows

import (
	"os"
	"runtime"
)

// Arch retrieves the runtime architecture. This might differ from the compile
// time architecture. hence GOARCH can't be used here.
func Arch() string {
	if runtime.GOARCH == "arm64" {
		return "arm64"
	}

	// See: https://learn.microsoft.com/en-us/archive/blogs/david.wang/howto-detect-process-bitness#detection-matrix
	switch os.Getenv("PROCESSOR_ARCHITECTURE") {
	case "AMD64":
		return "64bit"
	case "x86":
		if os.Getenv("PROCESSOR_ARCHITEW6432") == "AMD64" {
			return "64bit"
		}
		return "32bit"
	default:
		return "unsupported"
	}
}
