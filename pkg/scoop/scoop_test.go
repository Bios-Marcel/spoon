package scoop_test

import (
	"testing"

	"github.com/Bios-Marcel/spoon/pkg/scoop"
	"github.com/stretchr/testify/require"
)

func app(t *testing.T, name string) *scoop.App {
	defaultScoop, err := scoop.NewScoop()
	require.NoError(t, err)

	app, err := defaultScoop.GetAvailableApp(name)
	require.NoError(t, err)

	return app
}

func Test_ParseBin(t *testing.T) {
	t.Run("single string (single path entry)", func(t *testing.T) {
		app := app(t, "main/ripgrep")

		err := app.LoadDetails(scoop.DetailFieldBin)
		require.NoError(t, err)

		require.Len(t, app.Bin, 1)
		require.Equal(t, app.Bin[0], scoop.Bin{Name: "rg.exe"})
	})
	t.Run("top level array (path entries)", func(t *testing.T) {
		app := app(t, "main/go")

		err := app.LoadDetails(scoop.DetailFieldBin)
		require.NoError(t, err)

		// Order doesnt matter
		require.Len(t, app.Bin, 2)
		require.Contains(t, app.Bin, scoop.Bin{Name: "bin\\go.exe"})
		require.Contains(t, app.Bin, scoop.Bin{Name: "bin\\gofmt.exe"})
	})
	t.Run("nested array (multiple shims)", func(t *testing.T) {
		app := app(t, "extras/stash")

		err := app.LoadDetails(scoop.DetailFieldBin)
		require.NoError(t, err)

		// Order doesnt matter
		require.Len(t, app.Bin, 2)
		require.Contains(t, app.Bin, scoop.Bin{
			Name:  "stash-win.exe",
			Alias: "stash-win",
			Args:  []string{`-c "$dir\config\config.yml"`},
		})
		require.Contains(t, app.Bin, scoop.Bin{
			Name:  "stash-win.exe",
			Alias: "stash",
			Args:  []string{`-c "$dir\config\config.yml"`},
		})
	})
}

func Test_ParseArchitecture_Items(t *testing.T) {
	goApp := app(t, "main/go")

	err := goApp.LoadDetails(scoop.DetailFieldArchitecture)
	require.NoError(t, err)

	arch := goApp.Architecture
	require.Len(t, arch, 3)
	x386 := arch[scoop.ArchitectureKey32Bit]
	require.NotNil(t, x386)
	x686 := arch[scoop.ArchitectureKey64Bit]
	require.NotNil(t, x686)
	arm64 := arch[scoop.ArchitectureKeyARM64]
	require.NotNil(t, arm64)

	require.Len(t, x386.Items, 1)
	require.Len(t, x686.Items, 1)
	require.Len(t, arm64.Items, 1)

	require.Contains(t, x386.Items[0].URL, "386")
	require.NotEmpty(t, x386.Items[0].Hash)
	require.Empty(t, x386.Items[0].ExtractDir)

	require.Contains(t, x686.Items[0].URL, "amd64")
	require.NotEmpty(t, x686.Items[0].Hash)
	require.Empty(t, x686.Items[0].ExtractDir)

	require.Contains(t, arm64.Items[0].URL, "arm64")
	require.NotEmpty(t, arm64.Items[0].URL)
	require.NotEmpty(t, arm64.Items[0].Hash)
	require.Empty(t, arm64.Items[0].ExtractDir)
}
