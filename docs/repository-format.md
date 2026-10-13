# OVCPR v1 repository format

## Source layout

```text
packages/
  <package-id>/
    @versions/
      <normalized-version>.json
    @assets/
      <relative-asset-path>
```

Package IDs may contain slash-separated segments. A package ID can also be another package's prefix. Source `index.json` files are forbidden. Generated indexes exist only in the built site. `.dspm-repo` is local generator state and must remain untracked.

Paths require exact case and portable components. Links, hard links, submodules, LFS pointers, special files, case collisions, unknown reserved directories, and unrelated files inside `packages` are rejected.

## Schemas and parsing

The [manifest schema](../schemas/manifest-v1.schema.json), [versions index schema](../schemas/versions-index-v1.schema.json), and [packages index schema](../schemas/packages-index-v1.schema.json) define closed v1 objects. Procedural validation additionally enforces path identity, numeric ordering, compatibility, dependency graphs, immutability, network policy, archive projection, and asset byte equality.

JSON requires UTF-8 without a byte order mark. Duplicate decoded keys, unpaired surrogates, null values, unknown fields, and resource limit violations are rejected. Optional text fields preserve absence, explicit empty strings, and every language key. Language maps require the default key `_`.

Versions contain four nonnegative decimal integer components without leading zeros. Each component must fit a Go `int`. Numeric comparison determines order. Every published version remains eligible, including deprecated versions. The highest numeric version is latest.

## Metadata links

`packages/index.json` records the repository revision and links each exact versions index using SHA-512. Each versions index links exact manifest bytes. Each manifest links the exact archive file-byte SHA-512. Each manifest requires positive integer `downloadSize`, the exact compressed archive byte count. Validation compares this value with the downloaded file size. DSPM uses this field for remote information and download plans. The archive file digest differs from DSPM's installed content-tree digest.

Digest verification requires original bytes without JSON reformatting. Only the root index records the repository revision. Unrelated commits therefore do not change individual versions indexes.

## Dependencies and summaries

A dependency target requires the same package ID and `compatVersion <= target <= version`. Resolution selects the highest compatible provider without backtracking. Multiple versions of a package may coexist. Duplicate dependency IDs, self dependencies, missing providers, and cycles between complete identities are rejected.

Singer summaries include `id`, `name`, `interface`, `variant`, and a positive signed 64-bit integer `level`. Resource packages without singers contain `singers: []`. Source level values require integer literals without a fractional part or exponent. Remote manifests omit runtime level and internal package format version. Download validation checks those fields in the archive.

## Documentation assets

License and readme references are relative to the exact package's `@assets` directory. Safe custom mappings are permitted. Submission validation compares original asset bytes against archive resources. Multiple language keys may reference the same asset.

Ordinary DSPM installation compares publication metadata and installs documentation from the archive. It does not download or verify separately hosted Pages documentation assets.

## Immutability and generation

Accepted manifests and assets remain immutable except for deprecation state and reason. `deprecated: true` requires a nonblank `deprecationReason`. An absent or false deprecation state forbids a reason. Deprecation does not change resolution or installation eligibility.

The CI exporter reads Git blobs without checkout filters, newline conversion, LFS expansion, or export attributes. The builder preserves source manifest and asset bytes. Generated indexes use deterministic ordering, two-space indentation, UTF-8, LF, and a final newline. Identical source bytes produce identical package metadata bytes.
