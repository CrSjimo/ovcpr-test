# Contributing packages

1. Prepare the exact `.dspk` that will be available at a stable, anonymous HTTPS
   URL. Do not use login pages, credentials, expiring query strings or mutable
   `latest/download` URLs.
2. Run `dspm repo create`. It verifies the local archive and writes a manifest
   plus exact license/readme bytes; it does not contact the download URL.
3. Submit only new manifests and documentation assets. Dependencies may be added
   in the same PR. `foo` and `foo/bar` may coexist.
4. Keep package submissions separate from tools, schemas, policies, workflows,
   Go dependencies, CODEOWNERS and Git attributes. Mixed PRs fail.
5. All repository checks must pass against the current main merge candidate,
   and a core maintainer must approve. New commits dismiss old approval.

First-time fork contributors may need a maintainer to approve running Actions.
Conflicted PRs must be updated before GitHub can produce a merge candidate.
Pending, skipped and cancelled checks do not count as successful validation.
Fork jobs receive no secrets, use read-only permissions, and never execute
candidate scripts as part of package validation.

Once main accepts a version, its URL, digest, metadata and assets cannot change
or disappear. Deprecation can be enabled, its reason updated, or cancelled:
`deprecated: true` requires a nonblank `deprecationReason`; cancelling removes
the reason. Deprecation does not prevent download or resolution, and does not
change latest-version selection. Format-only rewrites of historical manifests
are rejected. Publish a new version for content or URL changes.

All package paths are case-sensitive and portable to Windows. Links, submodules,
LFS pointers, unknown reserved directories and committed generated indexes are
rejected. `/.dspm-repo/` is private generator state and must stay untracked.

The validator checks resource archive safety and package declarations. It does
not run package content, load inference engines or test synthesis quality.
