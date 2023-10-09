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
