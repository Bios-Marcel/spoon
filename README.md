# Spoon

A wrapper around `scoop`, replacing `scoop search` and offering an improved user
experience.

## Features

* Drop-In replacement for `scoop search`
  > Even faster than `scoop-search`!
* Tab completion for command, flags and packages

## Manual Installation

1. [Install scoop](https://scoop.sh/)
2. Install spoon
   ```ps
   go install github.com/Bios-Marcel/spoon/cmd/spoon@latest
   ```
3. Enable autocompletion by adding this to your powershell profile:
  ```ps
  spoon completion powershell | Out-String | Invoke-Expression
  ```

## CLI Progress

Rough overview of progress on the command line interface. Commands are
basically either fully fledged custom implementations or wrappers around scoop.
The wrappers are there to provide autocomplete or add feature on top that run
before / after the actual scoop commands.

**For now, the global mode isn't support for custom commands, as I personally
don't use that feature for now.**

Some commands will also probably never be fully completed. Such as alias, as I
don't see the value personally. However, you are free to contribute. The
commands are roughly ordered by priority.

All unknown commands are delegated to scoop by default.

| Command    | Implementation Type | Autocomplete | Changes                                                                  |
| ---------- | ------------------- | ------------ | ------------------------------------------------------------------------ |
| help       | Native              | ✅            |                                                                          |
| search     | Native              | ✅            | * Performance improvements<br/>* JSON output<br/> * Search configuration |
| install    | Wrapper             | ✅            |                                                                          |
| uninstall  | Wrapper             | ✅            | * Terminate running processes                                            |
| update     | Partially Native    | ✅            | * Now invokes `status` after updating buckets                            |
| bucket     | Wrapper             | ✅            | * `bucket rm` now supports multiple buckets to delete at once            |
| cat        | Native              | ✅            | * Alias `manifest`                                                       |
| status     | Native              | ✅            | * `--local` has been deleted (It's always local now)<br/>Shows outdated / installed things scoop didn't |
| info       | Wrapper             | ✅            |                                                                          |
| depends    | Native (WIP)        | ✅            | * Adds `--reverse/-r` flag<br/>* Prints an ASCII tree by default         |
| list       |                     |              |                                                                          |
| hold       |                     |              |                                                                          |
| unhold     |                     |              |                                                                          |
| reset      |                     |              |                                                                          |
| cleanup    |                     |              |                                                                          |
| create     |                     |              |                                                                          |
| shim       |                     |              |                                                                          |
| which      |                     |              |                                                                          |
| config     |                     |              |                                                                          |
| download   |                     |              |                                                                          |
| cache      |                     |              |                                                                          |
| prefix     |                     |              |                                                                          |
| home       |                     |              |                                                                          |
| export     |                     |              |                                                                          |
| import     |                     |              |                                                                          |
| checkup    |                     |              |                                                                          |
| virustotal |                     |              |                                                                          |
| alias      |                     |              |                                                                          |

## Search

The search here does nothing fancy, it simply does an offline search of
buckets, just like what scoop does, but faster. Online search is not supported
as I deem it unnecessary. If you want to search the latest, simply run
`scoop update; spoon search <app>`.

The search command allows plain output and JSON output. This allows use with
tools such as `jq` or direct use in powershell via Powershells builtin
`ConvertFrom-Json`.

