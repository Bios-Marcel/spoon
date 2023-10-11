package main

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"

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
		Run: func(cmd *cobra.Command, args []string) {
			yes, err := cmd.Flags().GetBool("yes")
			if err != nil {
				fmt.Println("error getting yes flag:", err)
				os.Exit(1)
			}
			if err := checkRunningProcesses(args, yes); err != nil {
				fmt.Println(err)
			}

			redirectedFlags, err := getFlags(cmd, "global", "purge")
			if err != nil {
				fmt.Println(err)
				os.Exit(1)
			}
			os.Exit(execScoopCommand("uninstall", append(redirectedFlags, args...)...))
		},
	}

	cmd.Flags().BoolP("global", "g", false, "Uninstall a globally installed app")
	cmd.Flags().BoolP("purge", "p", false, "Remove all persistent data")
	cmd.Flags().BoolP("yes", "y", false, "Decides whether questions arise or are automatically answered")

	return cmd
}

func checkRunningProcesses(args []string, yes bool) error {
	processes, err := wapi.ProcessList()
	if err != nil {
		return fmt.Errorf("error determining runing processes: %w", err)
	}

	appsDir, err := scoop.GetAppsDir()
	if err != nil {
		return err
	}

	var processPrefixes []string
	for _, arg := range args {
		processPrefixes = append(processPrefixes, strings.ToLower(filepath.Join(appsDir, arg)+"\\"))
	}

	var processesToKill []shared.Process
PROCESS_LOOP:
	for i := 0; i < len(processes); i++ {
		process := processes[i]
		for _, processPrefix := range processPrefixes {
			if strings.HasPrefix(strings.ToLower(process.Fullpath), processPrefix) {
				if i != len(processes)-1 {
					processes[i] = processes[len(processes)-1]
					processes = processes[:len(processes)-1]
				}
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
