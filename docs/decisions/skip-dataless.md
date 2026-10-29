# Do not read cloud placeholders

## Decision

`fstree` does not read a directory whose `st_flags` has `SF_DATALESS` (macOS File Provider placeholder: iCloud Drive, Dropbox, …) and returns it as skipped, like a directory on another filesystem. Sizing does not open a file with that flag. The pre-delete check therefore refuses an item that contains such a directory.

## Rationale

- Reading a dataless directory or opening a dataless file makes macOS download the content and blocks the call until the download ends. A scan was measured stuck for 8 minutes in one directory read under `~/Library/Mobile Documents`, and every scan downloaded the user's evicted iCloud items, using the disk space devclean is meant to free.
- A dataless item takes no space on this disk, so skipping it hides nothing reclaimable.
- The flag is in the `lstat` result the walk already has: no extra system call.

## Rejected alternatives

- **Excluding cloud folders by path** (`~/Library/Mobile Documents`, `~/Library/CloudStorage`) — misses Desktop and Documents synced to iCloud and providers that sync other folders.
- **Asking Foundation whether an item is ubiquitous** — needs cgo or an external command, and an iCloud item that is already downloaded reads without blocking, so the flag is the exact condition.
