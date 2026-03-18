# Configuration

> Configuration is not yet implemented. This documents the planned design.

## Location

```
~/.devclean/settings.json
```

## Planned Settings

```json
{
  "scan_paths": ["~"],
  "exclusions": [],
  "ecosystems": {
    "disabled": []
  },
  "thresholds": {
    "active": 7,
    "recent": 30,
    "stale": 90,
    "dormant": 90
  }
}
```

### Fields

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `scan_paths` | string[] | `["~"]` | Directories to scan |
| `exclusions` | string[] | `[]` | Paths to exclude from scanning |
| `ecosystems.disabled` | string[] | `[]` | Ecosystems to skip |
| `thresholds.active` | int | 7 | Days threshold for "active" status |
| `thresholds.recent` | int | 30 | Days threshold for "recent" status |
| `thresholds.stale` | int | 90 | Days threshold for "stale" status |
| `thresholds.dormant` | int | 90 | Days threshold for "dormant" status |
