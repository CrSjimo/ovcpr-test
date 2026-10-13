# Repository operations

## Release configuration

Public deployment requires a protected `main` branch containing tools, schemas, policies, workflows, and valid CODEOWNERS entries. Locked tool releases must be available before package submissions are accepted.

The Go module must pin a published DSPM public-library version with `go.sum` verification. Release dependency resolution must not rely on local paths or integration repositories. Release binaries must be built from the reviewed tool source and that dependency. `policy/toolchain.json` records validator and indexer versions, immutable HTTPS asset URLs, and SHA-256 digests for `linux-amd64`. The coordinator requires matching validator and indexer specifications. Tool binaries must expose the expected tool and library versions through `ovcpr version`.

Tool releases require tests, race detection, static analysis, and builds on supported platforms. The Linux deployment binary does not require CGO. A maintenance pull request must update dependency pins, release assets, checksums, and associated policy together. Workflow configuration must be reviewed against the protected baseline before deployment.

The following shell commands run from `tools/ovcpr` and require `OVCPR_RELEASE_VERSION` to contain the release identifier. The generated binary targets the Linux Actions runner. Native builds are required for local execution on other platforms.

```sh
go mod verify
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags "-X github.com/diffscope/ovcpr/internal/repository.ToolVersion=$OVCPR_RELEASE_VERSION" -o ../../dist/ovcpr-linux-amd64 ./cmd/ovcpr
go version -m ../../dist/ovcpr-linux-amd64
```

The release asset SHA-256 must match the tool lock. Release review requires `toolVersion`, `dspmVersion`, source revision, and build settings to match the approved inputs.

Tool distributions must include the [GPL license text](../tools/ovcpr/LICENSE) and applicable notices. Binary distribution requires the applicable Corresponding Source under GPL-3.0-or-later. Repository material outside the tool module uses Apache-2.0 as specified in the [licensing reference](../README.md#licensing).

## Branch protection

The required status checks are `repository-structure`, `package-validation`, `pages-build`, and `required-validation`. Protection requires an up-to-date merge candidate, at least one approving core review, CODEOWNERS review, and stale approval dismissal. Direct writes, force pushes, and branch deletion must be restricted. Protection must apply to administrators. Merge queues require the configured `merge_group` trigger.

Package validation reads the protected baseline's scripts, policies, and tool lock. Candidate commits are exported as Git blob data. Candidate scripts are not executed by package validation. Package and control changes require separate pull requests. Control changes require core review and branch protection because a same-repository workflow is not an independent organization-required workflow.

## Pages configuration

Pages must use GitHub Actions as its source. The `github-pages` environment must permit deployment only from protected `main`. Publication uploads the entire site and preserves the `/packages/` path. Deployment permission belongs to the separate deployment job.

All publication runs share one concurrency group with `cancel-in-progress: false`. Each run selects current `main` when the build starts, including manual reruns of earlier events. The root index and `build-info.json` record the selected `targetSha`. Build information and job summaries also record `eventSha`. Deployment metadata can associate the deployment with the event commit rather than the selected source commit.

The configured Pages URL is the site base URL. The package source URL appends `/packages/`. A custom domain requires repository Pages settings and DNS configuration. For `ovcpr.diffscope.org`, the DNS CNAME target is the repository owner's `<owner>.github.io` host. HTTPS enforcement requires an issued certificate. A committed CNAME file does not replace Pages settings. An owner-level custom domain may determine the published site URL.

## Deployment verification

The smoke job anonymously checks the root revision and every index and manifest digest link against the selected commit. Bounded retries permit CDN propagation. A successful deployment with failed smoke verification may already be public. The failure requires investigation and republication of current `main`. An earlier event must not restore obsolete metadata.

Deployment builds do not download previously accepted archives. Source manifests and documentation assets are copied from Git blobs. Acceptance checks establish archive and asset validity for new versions.

## Resource and network policy

| Resource | Default limit |
| --- | --- |
| Compressed archive | The limit is 4 GiB. |
| Expanded archive | The limit is 8 GiB. |
| Individual package file | The limit is 4 GiB. |
| Archive acquisition time | The limit is twenty minutes. |
| Repository manifests | The limit is 64 MiB. |
| Versions and dependency edges | The limits are 10,000 versions and 100,000 edges. |
| Documentation asset | The limit is 8 MiB. |
| Package documentation assets | The limit is 32 MiB. |
| Built site | The limit is 800 MiB. |

Downloads require public HTTPS destinations, validated DNS-to-connection binding, certificate verification, and allowed ports. Environment proxies are not inherited. Redirects, retries, idle time, elapsed time, and disk usage are bounded. `policy/download-policy.json` contains the configurable network and archive budgets.

## Maintenance policy

Ordinary submissions cannot bypass historical immutability. Emergency legal removals or accepted URL changes require a separate documented maintainer policy and reviewed tooling changes. Tool updates and package data changes remain separate maintenance operations.
