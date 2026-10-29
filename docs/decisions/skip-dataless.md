# Do not read cloud placeholders

## Decision

`fstree` does not read a directory whose `st_flags` has `SF_DATALESS` (a File Provider placeholder: iCloud Drive, Dropbox, …). It returns the directory as skipped, so the pre-delete check treats it like a directory on another filesystem ([one-filesystem.md](one-filesystem.md)).

## Rationale

- Reading a dataless directory makes macOS download its content and blocks the read until the download ends. One scan sat 8 minutes in a single read under `~/Library/Mobile Documents`, and scans downloaded evicted iCloud items, using the disk space devclean is meant to free.
- A placeholder takes no space on this disk, so skipping it hides nothing reclaimable.
- The flag is in the `lstat` result the walk already has, so the check adds no system call.
- Dataless files need no flag check: they have 0 allocated blocks (measured), and sizing never opens a file without blocks.

## Rejected alternatives

- **Excluding cloud folders by path** (`~/Library/Mobile Documents`, `~/Library/CloudStorage`): this misses Desktop and Documents synced to iCloud, and providers that sync other folders.
- **Asking Foundation whether an item is in iCloud**: this needs cgo or an external command. An iCloud item that is already downloaded reads without blocking, so the flag is the exact condition.
- **Turning materialization off for the process** (`setiopolicy_np` with `IOPOL_MATERIALIZE_DATALESS_FILES_OFF`): measured on macOS 27, an iCloud directory read still downloaded for 17 s and a file open still downloaded the file.
