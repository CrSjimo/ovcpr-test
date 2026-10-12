# Operations

Bootstrap an empty main containing tools, schemas, policies, workflows and valid
CODEOWNERS; publish locked tools before accepting the first package PR. Protect
main with strict up-to-date checks (`repository-structure`, `package-validation`,
`pages-build`, `required-validation`), one required approving core review,
CODEOWNERS review, stale approval dismissal, no force pushes or deletion, and no
ordinary direct writes. If merge queue is enabled, keep `merge_group` triggering.

`policy/toolchain.json` pins validator/indexer URLs, versions and SHA-256 for
linux-amd64. Those binaries import an exact dspm public-library module recorded
in their build info. Upgrade binaries, schemas and policy in a separate reviewed
maintenance PR. A package PR cannot choose its own validator: CI reads the
baseline's tool lock and scripts and treats the candidate only as Git blob data.
Control-plane workflow changes still depend on core review and branch protection;
a same-repository PR workflow is not an independent organization-required workflow.

Set Pages source to GitHub Actions. Restrict the `github-pages` environment to
protected main. Upload the entire site, preserving `/packages/`. All publication
jobs share one concurrency group with `cancel-in-progress: false`. Each surviving
run fixes **current main** when its build starts, including manual reruns of old
events. The root index and build-info record targetSha; build-info and the job
summary also record eventSha. Deploy-pages may associate deployment with eventSha.

After deployment, the smoke job anonymously verifies the root revision and every
manifest/index hash link against the fixed target, with bounded CDN propagation
retries. Deployment success followed by smoke failure means the new site may be
live but externally unverified; retry publishing current main. There is no silent
rollback to old metadata.

For production `ovcpr.diffscope.org`, configure the custom domain in Pages settings,
point its DNS CNAME at the repository owner's `<owner>.github.io`, wait for the
certificate and enable enforced HTTPS. A source CNAME file is insufficient.
Integration testing uses the test repository's github.io URL and does not change
production DNS.

Starting budgets are 4 GiB compressed, 8 GiB expanded, 4 GiB per package file,
20 minutes per archive, 64 MiB repository manifests, 10,000 versions, 100,000
edges, 8 MiB per asset, 32 MiB assets per package and 800 MiB site. Downloads are
public HTTPS with validated DNS-to-dial binding, normal TLS, no inherited proxy,
bounded redirects, retries, idle timeouts and disk-space checks. No historical
archive is downloaded during deployment. Emergency legal removals or URL changes
require a separately documented maintainer policy; ordinary PRs have no bypass.
