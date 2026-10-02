# Stay on the scan root's filesystem

## Decision

Finding, sizing and the pre-delete check walk through `fstree`, which stays on the root's filesystem (like `du -x`). A directory on another device, of unknown device, or unreadable is not entered and is returned as skipped. Path removal (including dry-run) is refused when `fstree` skipped any directory inside the item. To scan a mount, pass it as `--path`.

## Verification

```bash
go test ./internal/fstree/ -run 'OtherFilesystem|WithoutDevice|CheckOneFilesystem' -v
go test ./internal/cleaner/ -run 'TestRefusesUncheckedTree' -v
```

## Rationale

- Mounts under the root are not this machine's reclaimable disk. Example: Xcode CoreDevice's `~/Library/Developer/CoreDevice/DeviceFS`, measured at ~4 s per directory read.
- A recursive delete does not stop at mounts, so deleting an item that contains a skipped directory would remove content devclean never inspected.
- One shared traversal means sizing counts exactly what finding reports and deletion removes.

## Rejected alternatives

- **Separate traversal rules per stage** — finding, sizing and deletion could disagree about which files an item contains.
