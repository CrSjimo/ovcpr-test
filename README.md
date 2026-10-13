# OpenVPI Community Package Repository

OpenVPI Community Package Repository (OVCPR) is a GitHub-hosted community package repository for DiffScope Package Manager. It stores OVCPR v1 manifests and documentation assets. Package archives remain at stable public HTTPS URLs controlled by publishers. GitHub Pages serves generated metadata under `/packages/`.

## Build requirements

The validator and index builder are a CGO-free Go module in `tools/ovcpr`. CI coordination scripts are in `tools`. Builds require Go 1.26.1 or later.

```sh
cd tools/ovcpr
go build -o ../../dist/ovcpr ./cmd/ovcpr
go test ./...
go test -race ./...
go vet ./...
```

The commands below require `dspm` and `ovcpr` on the executable search path. Repository paths are relative to the invocation directory.

## Package publication

```sh
dspm repo create --file ./release.dspk --repository ./community-repository --download-url https://example.org/releases/1/release.dspk
ovcpr validate --repository ./community-repository --baseline ./protected-baseline --network-policy ./community-repository/policy/download-policy.json --cache-dir ./temporary-downloads --json
```

Submission generation validates the local archive without contacting the publication URL. OVCPR submission validation checks the complete candidate dependency graph and historical immutability. Every new version requires an actual archive download, exact file SHA-512, archive validation, metadata projection, and documentation asset byte comparison.

Previously accepted versions are not downloaded again by submission CI or Pages builds. Deprecation does not remove a version or exclude it from resolution. `structure` and `validate --offline` provide metadata validation rather than publication approval. Successful publication validation requires exit status zero and a successful complete JSON report.

## Documentation

- The [contribution reference](docs/contributing.md) specifies archive, submission, and review requirements.
- The [repository format](docs/repository-format.md) specifies source layout, metadata links, and immutability.
- The [tool reference](docs/tool-reference.md) specifies commands and validation reports.
- The [operations reference](docs/operations.md) specifies release configuration, branch protection, tool updates, and Pages deployment.

## Repository control

Package submissions contain only manifests and documentation assets. Changes to tools, dependencies, schemas, policies, workflows, ownership rules, or Git attributes require separate maintenance pull requests. Package validation uses scripts and checksum-locked tools from the protected baseline. The public DSPM libraries provide shared package format and dependency semantics.

## Licensing

Repository-owned material is licensed under [Apache-2.0](LICENSE), except for the Go module in `tools/ovcpr`. The validator, index builder, and associated source files in that module are licensed under [GPL-3.0-or-later](tools/ovcpr/LICENSE). The [tool licensing reference](tools/ovcpr/README.md#licensing) specifies that scope.

The Apache-2.0 scope includes repository workflows, schemas, project documentation, `tools/ci.py`, and `tools/smoke.py`. Package archives and publisher-supplied documentation assets retain their respective licenses. Repository licensing does not relicense those materials or third-party dependencies.
