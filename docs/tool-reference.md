# OVCPR tool reference

## Commands

`ovcpr` reads repository data and returns one JSON report on stdout. Diagnostics from argument parsing may also appear on stderr. The `--json` flag is accepted for explicit invocation. JSON is the report format with or without that flag.

```sh
ovcpr structure --repository ./candidate --baseline ./baseline --json
ovcpr validate --repository ./candidate --baseline ./baseline --network-policy ./baseline/policy/download-policy.json --cache-dir ./temporary-downloads --json
ovcpr build --repository ./candidate --revision <full-commit-sha> --output ./site --json
ovcpr export --repository ./git-repository --revision <full-commit-sha> --output ./exported --json
ovcpr classify --repository ./git-repository --baseline-revision <full-base-sha> --candidate-revision <full-candidate-sha> --json
ovcpr version --json
```

| Command | Behavior |
| --- | --- |
| `structure` | Validation includes source layout, manifest parsing, complete dependency graphs, and baseline immutability without archive downloads. |
| `validate` | Validation additionally downloads and verifies every version absent from the baseline. |
| `build` | Generation validates source data and writes a complete site to a new output directory. |
| `export` | Export reads a fixed Git commit directly as blobs and writes repository data to a new directory. |
| `classify` | Classification compares fixed commits and rejects mixed package and control changes. |
| `version` | The result records `toolVersion` and `dspmVersion`, including the dependency replacement if present. |

`validate --offline` has the same validation scope as `structure`. An omitted baseline treats every candidate version as new and cannot establish historical immutability. Publication validation requires a trusted baseline and successful online verification. `--candidate-revision` and `--baseline-revision` identify exported data in reports. These labels do not independently establish the data's provenance. CI supplies exports from fixed commits.

`--network-policy` selects trusted policy JSON. An omitted policy uses built-in defaults. `--cache-dir` selects temporary archive storage. An omitted directory uses an automatically removed temporary directory. Downloaded validation archives are removed after verification.

Archive acquisition permits at most three attempts. Transient response read failures and retryable HTTP statuses permit retries. Resource limit violations and local write or synchronization failures terminate acquisition without another download. Disk space prechecks use the known archive size without a fixed reserve. Write errors remain authoritative because available space can change during acquisition.

`build` and `export` require `--output`. Existing output directories are rejected. Build output must not overlap source data. `export` and `classify` require complete commit hashes rather than branch names.

## Report envelope

Every report contains `protocolVersion: 1`, `command`, `ok`, and `diagnostics`. Success contains `data`. Failure contains `error` and available `partialData`. Error objects contain `code`, `message`, and optional `path` and `details`.

Successful execution requires exit status zero and `ok: true`. A partial report, successful structural check, or incomplete archive count does not establish publication acceptance. Consumers must classify failures by error code rather than diagnostic wording.

## Validation payload

Validation data records scope, source revisions, baseline status, tool version, DSPM module version, policy version, manifest counts, archive counts, asset counts, dependency edges, new identities, and completed archive results.

Online validation uses `scope: new-versions`. Successful acceptance requires `baselineChecked: true` and `requiredArchives == verifiedArchives == len(newVersions)`. A deprecation-only change has zero required archives. Offline validation uses `scope: offline` and does not establish archive URL availability or archive content validity.

Each completed archive result records identity, `downloadSha512`, `contentTreeSha512`, and `urlVerified`. The archive file digest and content-tree digest have distinct meanings. `verifiedAssets` counts verified resource mappings, including language mappings that share the same physical asset.

## Error categories

| Code | Condition |
| --- | --- |
| `INVALID_REPOSITORY_LAYOUT`, `INVALID_MANIFEST` | Source paths, file types, identities, or manifests are invalid. |
| `IMMUTABLE_VERSION_CHANGED` | An accepted manifest or asset changes outside permitted deprecation fields. |
| `MIXED_CONTROL_AND_PACKAGE_CHANGE` | A change combines package data and repository control files. |
| `DEPENDENCY_ERROR` | Complete dependency graph validation fails. |
| `DOWNLOAD_POLICY_VIOLATION`, `DOWNLOAD_FAILED` | Archive acquisition violates policy or fails. |
| `DOWNLOAD_HASH_MISMATCH` | Archive file bytes do not match the manifest digest. |
| `INVALID_PACKAGE_ARCHIVE` | Archive safety, package parsing, or projection fails. |
| `PACKAGE_METADATA_MISMATCH`, `ASSET_CONTENT_MISMATCH` | Archive metadata or resource bytes differ from publication data. |
| `RESOURCE_LIMIT_EXCEEDED` | A repository, archive, site, or available storage budget is exceeded. |
| `INTERNAL_ERROR` | A failure has no more specific tool classification. |
