

The goal is to provide a fast and easy to use addition to scoop.

The main inspiration is the fact that scoop doesn't do proper cleaning of
cache and installed apps and the search is slow as hell, there's also no
autocompletion.

Additionally, this library can be used as a go package, providing spoon
functionallity and some low level scoop functionallity.

## Setup

1. [Install scoop](https://scoop.sh/)
2. Install spoon
   ```ps
   go install github.com/Bios-Marcel/spoon/cmd/spoon@latest
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
| help       | Custom              | ✅            |                                                                          |
| search     | Custom              | ✅            | * Performance improvements<br/>* JSON output<br/> * Search configuration |
| install    | Wrapper             | ✅            |                                                                          |
| uninstall  | Wrapper             | ✅            | * Terminate running processes                                            |
| update     | Wrapper             | ✅            |                                                                          |
| bucket     | Wrapper             | ✅            | * `bucket rm` now supports multiple buckets to delete at once            |
| cat        | Wrapper             | ✅            | * Alias `manifest`                                                       |
| status     | Wrapper             | ✅            |                                                                          |
| info       | Wrapper             | ✅            |                                                                          |
| list       |                     |              |                                                                          |
| hold       |                     |              |                                                                          |
| unhold     |                     |              |                                                                          |
| reset      |                     |              |                                                                          |
| cleanup    |                     |              |                                                                          |
| create     |                     |              |                                                                          |
| depends    |                     |              |                                                                          |
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
as i deem it unnecessary. If you want to search the latest, simply run
`scoop update; spoon search <app>`.

The search command allows plain output and JSON output. This allows use with
tools such as `jq` or direct use in powershell via Powershells builtin
`ConvertFrom-Json`.

WIP:

* Proper output, error handling and process exit codes

## Cleanup

WIP:

* Cleanup with retention count
  > Scoop allows you to cleanup a single app or all apps, but it doesn't allow
  > you to specify a retention count. This is a problem, because you might want
  > to keep the last 3 versions of an app. This is especially useful for
  > executing some kind of cleanup task.
  >
  > So you'd execute:
  > ```ps
  > spoon cleanup --retention 3 vscode
  > spoon cleanup --retention 3 *
  > ```
  >
  > While the default would be to keep the last 2 versions, as itd always wise
  > to keep the last version and the one before that. You may aswell force it
  > to 1 though.
  * No plan to support global apps for now.

## Install / Update

WIP

Scoop wastes time by not parallelizing the installation of apps with
the download of installation files.

Since scoop probably does a smart thing or two when installing, one way to do
this in a way to not replicate too much of scoops logic, would be to populate
the cache and then trigger `scoop install`.

### Installing specific versions

Scoop currently allows something such as

```ps
scoop install tokei@12.1.1
```

This creates an autogenerated manifest in `workspaces`. This manifest is
referred to in `12.1.1/install.json`. This has the downside that you can't
reset the pacakage back to its bucket version easily (or its not apparent how)
and the package is neither hold nor can it be unhold.

I've tested manually changed the `install.json` to refer to the bucket version
again and holding the package. Therefore, this should also be possible
programmatically and we can make proper use of the hold mechanism.

It's also possible to check old versions of the manifests via git. So if we
can find an old version, we can use that, otherwise generate a manifest and
pray.

Scoop also auto installs with auto generated manifests without asking the user.
While there's a warning, the only way to interfere with the installation, is to
be fast enough, with Ctrl-C.

### Downloading

Downloading should use up as much bandwidth as possible. If we download from
URL 1 with 2Mbit/s, but are on a 100Mbit/s connection, we should download from
more than one URL at once. This prevents unnecessary wait for a single download.

## Uninstalling

Prompt for closing running instances and add -y / --yes for automatically
confirming such questions and go defensive routes where not destructive.
