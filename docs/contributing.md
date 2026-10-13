# Package contributions

## Archive requirements

A release archive must have a stable, anonymous HTTPS URL. Login pages, embedded credentials, expiring query strings, and mutable `latest/download` URLs are rejected. The exact archive must remain available after acceptance. Archive changes require a new package version.

The publisher must have distribution rights for the package and its documentation. Validation checks archive safety and package declarations. It does not execute package content, load inference engines, or evaluate synthesis quality.

## Submission generation

```sh
dspm repo create --file ./release.dspk --repository ./community-repository --download-url https://example.org/releases/1/release.dspk
```

`repo create` verifies the local archive and generates its manifest and raw license and readme assets. The command does not contact the download URL. The private `.dspm-repo` directory contains generator state and must remain untracked. Generated source indexes must not be committed.

New manifests and required dependency packages may share a pull request. Package IDs such as `foo` and `foo/bar` may coexist. Paths are case-sensitive and portable to Windows. Links, submodules, LFS pointers, unknown reserved directories, and generated indexes are rejected.

## Validation and review

Package data changes must remain separate from tools, Go dependencies, schemas, policies, workflows, CODEOWNERS, and Git attributes. Mixed pull requests are rejected. The [tool reference](tool-reference.md) specifies local validation.

All required checks must pass for the current main merge candidate. A core maintainer must approve the submission. New commits dismiss stale approvals. Conflicts require an updated merge candidate. Pending, skipped, failed, and canceled checks do not establish successful validation.

First-time fork contributors may require maintainer approval for Actions execution. Package validation uses read-only permissions, receives no secrets, and does not execute candidate scripts.

## Historical immutability

Accepted version URLs, digests, metadata, and assets are immutable. Ordinary pull requests cannot remove accepted versions or assets. Formatting-only rewrites of accepted manifests are rejected. Content or URL changes require a new version.

Deprecation may be enabled, updated, or canceled. `deprecated: true` requires a nonblank `deprecationReason`. Canceling deprecation removes the reason. Deprecation does not prevent resolution or installation and does not change latest-version selection.

## Licensing

Contributions to repository-owned material use the license of the destination component. The `tools/ovcpr` module uses GPL-3.0-or-later. Other repository-owned material uses Apache-2.0. The [repository licensing reference](../README.md#licensing) defines the component boundaries.

Package archives and publisher-supplied documentation assets retain their respective licenses. Publication requires redistribution rights for hosted assets. Existing copyright and license notices must be preserved.

Published manifests require `downloadSize`, the exact compressed archive byte count. Submission generation records this field. Online validation requires both the declared byte count and SHA-512 digest to match the archive.
