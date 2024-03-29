# Spoon

A wrapper around `scoop`, aiming to be a full drop-in replacement, but still
relying on the existing community work in form of buckets.

## Highlighted Features

* More thorough `scoop search`
* Better performance (Varies from command to command)
* Additional features
  * Tab completion for commands, flags and packages
  * Common command aliases
    > For example no need to gues whether it's `uninstall`, `rm` or `remove`.
  * New commands
    * `spoon shell`, it's kinda like `nix-shell`

For a more detailed list of changes in comparison to scoop, check the table
below.

## Breaking Changes

For now, `--global` isn't implemented anywhere and I am not planning to do so as
of now. If there's demand in the future, I will consider.

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

Unknown / Unimplemented commands are delegated to scoop.

| Command    | Implementation Type | Changes                                                                  |
| ---------- | ------------------- | ------------------------------------------------------------------------ |
| help       | Native              |                                                                          |
| search     | Native              | * Performance improvements<br/>* JSON output<br/> * Search configuration |
| install    | Wrapper             |                                                                          |
| uninstall  | Wrapper             | * Terminate running processes                                            |
| update     | Partially Native    | * Now invokes `status` after updating buckets                            |
| bucket     | Partially Native    | * `bucket rm` now supports multiple buckets to delete at once            |
| cat        | Native              | * Alias `manifest`<br/>* Allow getting specific manifest versions        |
| status     | Native              | * `--local` has been deleted (It's always local now)<br/>* Shows outdated / installed things scoop didn't (due to bugs) |
| info       | Wrapper             |                                                                          |
| depends    | Native (WIP)        | * Adds `--reverse/-r` flag<br/>* Prints an ASCII tree by default         |
| list       |                     |                                                                          |
| hold       |                     |                                                                          |
| unhold     |                     |                                                                          |
| reset      |                     |                                                                          |
| cleanup    |                     |                                                                          |
| create     |                     |                                                                          |
| shim       |                     |                                                                          |
| which      |                     |                                                                          |
| config     |                     |                                                                          |
| download   |                     |                                                                          |
| cache      |                     |                                                                          |
| prefix     |                     |                                                                          |
| home       |                     |                                                                          |
| export     |                     |                                                                          |
| import     |                     |                                                                          |
| checkup    |                     |                                                                          |
| virustotal |                     |                                                                          |
| alias      |                     |                                                                          |

## Search

The search here does nothing fancy, it simply does an offline search of
buckets, just like what scoop does, but faster. Online search is not supported
as I deem it unnecessary. If you want to search the latest, simply run
`scoop update; spoon search <app>`.

The search command allows plain output and JSON output. This allows use with
tools such as `jq` or direct use in powershell via Powershells builtin
`ConvertFrom-Json`.

