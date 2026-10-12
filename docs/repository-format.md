# OVCPR v1

`schemas/` contains the three closed v1 wire schemas. Procedural validation also
checks path identity, numeric versions, compatibility, graph correctness,
immutable history, network policy, archive projection and asset bytes.

Source layout is `packages/<id>/@versions/<version>.json` and
`packages/<id>/@assets/<relative-path>`. Source `index.json` files are forbidden.
Indexes exist only in the built site. Versions have four arbitrary-precision
nonnegative integer components, without leading zeros. Every published version,
including deprecated versions, remains a candidate. The highest numeric version
is latest.

A dependency target is satisfied when `compatVersion <= target <= version` for
the same package ID. Select the highest compatible version without backtracking.
Multiple versions of one ID may coexist. Duplicate dependency IDs, self
dependencies, missing providers and cycles of complete identities fail.

The package index links the exact bytes of every versions index using SHA-512;
versions indexes link exact manifest bytes. Manifests link the archive's exact
file-byte SHA-512, which differs from dspm's installed content-tree hash. Do not
reformat JSON before checking these links. Only the root index carries the Git
repository revision, so unrelated commits do not invalidate each package index.

Strict UTF-8 JSON rejects BOMs, duplicate decoded keys, invalid Unicode scalar
escapes, nulls, unknown fields and excess budgets. Optional Text fields preserve
absence, explicit empty strings and every language key. Singer summaries include
ID, name, interface, variant and positive decimal-string level. Pure resource
packages have `singers: []`. Runtime level and internal package format version
are verified after download and are absent from remote manifests.

License/readme references are relative to that exact package's `@assets`.
Manual safe mappings are allowed. Validation compares actual resource bytes;
normal dspm installation compares metadata and installs archive resources without
downloading Pages documentation. The two scopes are reported separately.

The builder and CI exporter use Git blobs, bypassing checkout filters, CRLF
conversion, LFS and export attributes. Generated files use stable order, two
spaces, UTF-8, LF and a final newline. Building identical inputs twice produces
identical package metadata bytes.
