The goal is to provide a fast and easy to use addition to scoop.

The main inspiration is the fact that scoop doesn't do proper cleaning of
cache and installed apps and the search is slow as hell.

WIP:

* Autocomplete (spoon and scoop)
* Delegation to scoop for install, uninstall, update, status, ...


## Setup

1. [Install scoop](https://scoop.sh/)
2. Install spoon
   ```ps
   go install github.com/Bios-Marcel/spoon@latest
   ```
3. Option A: Enable Completion
   1. Generate completion `spoon completion powershell > spoon_completion.ps1`
   2. Move file into place of your liking
   3. Source the file in your powershell profile
3. Option B: EnableCompletion; Since way 3 requires to update the completion
   profile after spoon updates, you can alternatively add this to your profile:
  ```ps
  spoon completion powershell | Out-String | Invoke-Expression
  ```

## Search

The search here does nothing fancy, it simply does an offline search of
buckets, just like what scoop does, but faster. Online search is not supported
as i deem it unnecessary. If you want to search the latest, simply run
`scoop update; spoon search <app>`.

The search command allows plain output and JSON output. This allows use with
tools such as `jq` or direct use in powershell via Powershells builtin
`ConvertFrom-Json`.

WIP:

* Proper output, error handling and process exit codes

## Cleanup

WIP
