package windows_test

import (
	"testing"

	"github.com/Bios-Marcel/spoon/internal/windows"
	"github.com/stretchr/testify/require"
)

func Test_ParsePath(t *testing.T) {
	t.Parallel()

	path := windows.ParsePath(`C:\path_a;"C:\path_b";"C:\path_;";C:\path_c`)
	require.Equal(t, []string{`C:\path_a`, `C:\path_b`, `C:\path_;`, `C:\path_c`}, []string(path))
	require.Equal(t, `"C:\path_a";"C:\path_b";"C:\path_;";"C:\path_c"`, path.String())
}
