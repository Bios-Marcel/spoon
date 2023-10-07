The goal is to provide a fast and easy to use addition to scoop.

The main inspiration is the fact that scoop doesn't do proper cleaning of
cache and installed apps and the search is slow as hell.

WIP:

* Autocomplete (spoon and scoop)
* Delegation to scoop for install, uninstall, update, status, ...

## Search

The search here does nothing fancy, it simply does an offline search of
buckets, just like what scoop does, but faster. Online search is not supported
as i deem it unnecessary. If you want to search the latest, simply run
`scoop update; spoon search <app>`.

The search command allows plain output and JSON output. This allows use with
tools such as `jq` or direct use in powershell via Powershells builtin
`ConvertFrom-Json`.

WIP:

* Make the search more configurable, allowing you to specify where to search and how strict to be.
* Proper output, error handling and process exit codes

## Cleanup

WIP
