package main

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Bios-Marcel/spoon/internal/windows"
	"github.com/Bios-Marcel/spoon/pkg/scoop"
	wapi "github.com/iamacarpet/go-win64api"
	"github.com/iamacarpet/go-win64api/shared"
	"github.com/rodaine/table"
	"github.com/spf13/cobra"
)

func uninstallCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "uninstall",
		Short: "Uninstall an installed pacakge (Data is kept by default)",
		Aliases: []string{
			"remove",
			"delete",
			"rm",
		},
		Args:              cobra.MinimumNArgs(1),
		ValidArgsFunction: autocompleteInstalled,
		RunE: RunE(func(cmd *cobra.Command, args []string) error {
			yes, err := cmd.Flags().GetBool("yes")
			if err != nil {
				return fmt.Errorf("error getting yes flag: %w", err)
			}
			defaultScoop, err := scoop.NewScoop()
			if err != nil {
				return fmt.Errorf("error getting default scoop: %w", err)
			}

			if err := checkRunningProcesses(defaultScoop, args, yes); err != nil {
				return fmt.Errorf("error checking running processes: %w", err)
			}

			// FIXME 3 funcs: FindInstalledApp, FindInstalledApps,
			// InstalledApps. The later returns all of them, returning
			// everything instead of finding something.
			for _, arg := range args {
				app, err := defaultScoop.FindInstalledApp(args[0])
				if err != nil {
					return err
				}

				// FIXME Is this good? What does scoop do?
				if app == nil {
					fmt.Printf("App '%s' is not intalled.\n", arg)
					continue
				}

				// FIXME We need to make the loading stuff less annoying. Can we
				// have a special optimisation path / package so that we can
				// still cover stuff such as search?
				if err := app.LoadDetails(scoop.DetailFieldsAll...); err != nil {
					return fmt.Errorf("error loading app details: %w", err)
				}

				// FIXME This uninstalls the current version and then deletes
				// all installed versions via file-deletion. Should this be part
				// of the API and do we need to be more careful here?
				if err := defaultScoop.Uninstall(app, app.Architecture); err != nil {
					return fmt.Errorf("error uninstalling '%s': %w", arg, err)
				}
				if err := windows.ForceRemoveAll(filepath.Join(defaultScoop.AppDir(), app.Name)); err != nil {
					return fmt.Errorf("error cleaning up installation of '%s': %w", arg, err)
				}
			}

			// redirectedFlags, err := getFlags(cmd, "global", "purge")
			// if err != nil {
			// 	fmt.Println(err)
			// 	os.Exit(1)
			// }
			// os.Exit(execScoopCommand("uninstall", append(redirectedFlags, args...)...))
			return nil
		}),
	}

	cmd.Flags().BoolP("global", "g", false, "Uninstall a globally installed app")
	cmd.Flags().BoolP("purge", "p", false, "Remove all persistent data")
	cmd.Flags().BoolP("yes", "y", false, "Decides whether questions arise or are automatically answered")

	return cmd
}

func checkRunningProcesses(scoop *scoop.Scoop, args []string, yes bool) error {
	// FIXME Replace with custom code?
	processes, err := wapi.ProcessList()
	if err != nil {
		return fmt.Errorf("error determining runing processes: %w", err)
	}

	var processPrefixes []string
	for _, arg := range args {
		processPrefixes = append(processPrefixes,
			strings.ToLower(filepath.Join(scoop.AppDir(), arg)+"\\"))
	}

	var processesToKill []shared.Process
PROCESS_LOOP:
	for i := 0; i < len(processes); i++ {
		process := processes[i]
		for _, processPrefix := range processPrefixes {
			if strings.HasPrefix(strings.ToLower(process.Fullpath), processPrefix) {
				processes[i], processes = processes[len(processes)-1], processes[:len(processes)-1]
				i--
				processesToKill = append(processesToKill, process)
				continue PROCESS_LOOP
			}
		}
	}

	if len(processesToKill) > 0 {
		fmt.Println("There are still active processes")
		processTable := table.New("ProcessId", "ProcessName", "File")

		for _, process := range processesToKill {
			processTable.AddRow(process.Pid, process.Executable, process.Fullpath)
		}

		processTable.Print()

		kill := yes
		if !kill {
			fmt.Print("Attempt to terminate the processes?\n\r(yes/no)? ")
			if scanner := bufio.NewScanner(os.Stdin); scanner.Scan() {
				text := strings.ToLower(scanner.Text())
				if text == "yes" || text == "y" {
					kill = true
				}
				fmt.Print("\n")
			}
		}

		if kill {
			for _, process := range processesToKill {
				bool, err := wapi.ProcessKill(uint32(process.Pid))
				if err != nil {
					return fmt.Errorf("error killing process: %w", err)
				}

				if !bool {
					fmt.Println("Couldn't kill ", process.Executable)
				}
			}
		}
	}

	return nil
}
