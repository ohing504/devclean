# Hugging Face shared blobs follow huggingface_hub's reference manifests

## Decision

When the hub cache has a shared blob store (`hub/blobs/.huggingface-shared-blobs` containing `1`), the llm scanner reads each payload's `<hash>.refs` and keeps only entries that are still repo `blobs/<etag>` symlinks pointing at that payload. These rules mirror huggingface_hub `utils/_shared_blobs.py` (read in 1.33.0):

- **Size**: a payload referenced by one model is added to that model's `size`; one referenced by several repos goes to `BlobShares`, counted once in totals when every reference is a listed model.
- **Delete**: after a model directory is removed, its payloads are swept: under the payload's `<hash>.lock` flock, a payload with a readable manifest and no valid reference is removed with its manifest, through the same Trash or permanent removal as the model.
- **Never delete** a payload whose manifest is missing or unreadable, or whose lock is held; devclean does not wait for the lock.
- **Unreferenced payloads** form one result, deleted permanently by an `api` method.

## Verification

```bash
go test ./internal/scanner/ -run 'TestLLMScanner_HF' -v
go test ./internal/cleaner/ -run 'Release' -v
```

Measured 2026-10-16 on a hub with two models sharing one payload: `scan --eco llm` reported 9.1 MB before, 8,358,182,912 B after; `du -sk` of the hub was 8,358,203,392 B (the rest is manifests, locks and directories).

## Rationale

- Model directories hold only symlinks, and [never following symlinks](symlink-no-follow.md) leaves the payloads in no result: GB of weights went unreported, and deleting a model freed nothing.
- The manifest names every repo that uses a payload, datasets and spaces included, so a payload is deleted only when huggingface_hub itself would delete it.
- Holding the same flock keeps a sweep from racing a download that is adding a reference.
- The `hf` CLI is often not on `PATH` (pip installs it into a project venv), so the sweep runs in-process.

## Rejected alternatives

- **Report `hub/blobs` as one result** — no per-model choice, and deleting it makes every remaining model re-download.
- **Delete through `hf cache rm`** — fails wherever `hf` is not on `PATH`.
- **Delete the model directory only and report leftovers on the next scan** — reclaiming a model takes two cleans.
- **Follow the model's symlinks** — counts a payload shared by two models in both and would delete it while the other still uses it.
