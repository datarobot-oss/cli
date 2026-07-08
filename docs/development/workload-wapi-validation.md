# Workload `.wapi/` validation reference

This page documents the struct-tag validation used for workload local state stored in `.wapi/` (for example `config.json` and `manifest.json`).

Validation is implemented with `github.com/go-playground/validator/v10` and centralized in `internal/workload/wapi/validate.go`.

## Custom validator tags

The workload `.wapi/` validation uses these custom tags (registered in `internal/workload/wapi/validate.go`):

- **`dr_id`**&mdash;non-empty identifier (max 256 chars) with no `/`, `\`, or `..` (safe for filesystem paths).
- **`dr_nonempty_ptr`**&mdash;if a `*string` is non-nil, the pointed-to string must not be `""` (rejects JSON `""` where `null` was intended).
- **`dr_sha256hex`**&mdash;exactly 64 lowercase hex digits, no `0x`/`0X` prefix (matches `hex.EncodeToString` output from the sync engine).

## Built-in tags used

The structs also use standard `validator/v10` tags:

- **`required`**&mdash;field must be present and non-zero (e.g. non-empty string, non-zero `time.Time`).
- **`omitempty`**&mdash;skip remaining tag checks when the field is empty (nil pointer, empty string, zero number).
- **`eq=1`**&mdash;integer field must equal `1` (only supported manifest schema version).
- **`gte=0`**&mdash;numeric field must be greater than or equal to zero.
- **`len=64`**&mdash;string must be exactly 64 characters long.
- **`dive`**&mdash;validate each value inside a map/slice (used so every `files` entry runs `FileMeta` rules).

## JSON fields (by struct)

### `Config` / `InitOptions`

- **`artifactId`** (`Config`, `InitOptions`)&mdash;`required,dr_id`
- **`catalogId`** (`Config` as `*string`)&mdash;`omitempty,dr_nonempty_ptr,dr_id`
- **`catalogId`** (`InitOptions` as `string`)&mdash;`omitempty,dr_id`
- **`lastSyncedVersionId`** (`Config` as `*string`)&mdash;`omitempty,dr_nonempty_ptr,dr_id`
- **`lastSyncedVersionId`** (`InitOptions` as `string`)&mdash;`omitempty,dr_id`
- **`createdAt`** (`Config`)&mdash;`required`
- **`cliVersion`** (`Config`)&mdash;`required`

### `Manifest` / `FileMeta`

- **`version`** (`Manifest`)&mdash;`eq=1`
- **`syncedVersionId`** (`Manifest` as `*string`)&mdash;`omitempty,dr_nonempty_ptr,dr_id`
- **`syncedAt`** (`Manifest`)&mdash;validated by a cross-field rule (see below).
- **`files`** (`Manifest`)&mdash;values validated via `dive`; keys are validated with `fileops.SafeRelPath`.
- **`hash`** (`FileMeta`)&mdash;`required,dr_sha256hex`
- **`size`** (`FileMeta`)&mdash;`gte=0`

## Cross-field rules (not tags)

Some rules are enforced in code in addition to struct tags:

- **Catalog required with version** (`Config`, `InitOptions`)&mdash;if `lastSyncedVersionId` is set, `catalogId` must also be set.
- **`syncedAt` ↔ `syncedVersionId`** (`Manifest`)&mdash;if either is set, both must be set; both unset is valid for a fresh init.
- **Safe file paths** (`Manifest.files` keys)&mdash;each key must pass `fileops.SafeRelPath` (rejects traversal, absolute paths, and backslashes).

## Where validation runs

- **`LoadConfig`**&mdash;calls `validateConfig` and returns a `*CorruptedError` on failure.
- **`LoadManifest`**&mdash;calls `validateManifest` and returns a `*CorruptedError` on failure.
- **`Initialize`**&mdash;calls `validateInitOptions` at the start and returns a plain `error` on failure (before `.wapi/` is created).

Validation is intentionally **not** run on `SaveConfig` / `SaveManifest` (trusted writers).

## Note on SemVer

Semantic version validation in this repo is handled separately via `github.com/Masterminds/semver/v3` (for example `internal/tools/validation.go`), not via `validator/v10`.

