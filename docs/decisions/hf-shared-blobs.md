# Hugging Face shared blobs follow their reference manifests

## Decision

Shared blobs are sized and deleted by their `<hash>.refs` manifests with huggingface_hub's rules (`utils/_shared_blobs.py`): a reference counts only while its symlink still points at the blob, and a blob whose manifest is unreadable or whose `<hash>.lock` is held is never deleted.

## Verification

```bash
go test ./internal/scanner/ -run 'TestLLMScanner_HF' -v
go test ./internal/cleaner/ -run 'Release' -v
```

## Rationale

- Model directories hold only symlinks, so without the store GB of weights went unreported and deleting a model freed nothing.
- The manifest lists every repo using a blob, datasets included, so devclean deletes only what huggingface_hub would.

## Rejected alternatives

- **`hub/blobs` as one result** — no per-model choice; deleting it makes every model re-download.
- **`hf cache rm`** — `hf` is often not on `PATH`.
- **Follow the symlinks** — counts a shared blob twice and deletes it while another model uses it.
