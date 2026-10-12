# OVCPR

OVCPR is a GitHub-hosted community package source for DiffScope Package Manager.
The Go validator/indexer module is contained in `tools/ovcpr/`; CI coordination scripts are in `tools/`.
It stores immutable OVCPR v1 package manifests and documentation. Release
archives stay at publishers' public HTTPS URLs. GitHub Pages serves generated
indexes under `/packages/`.

Build the CGO-free tool with Go 1.26.1 or newer:

```sh
cd tools/ovcpr
go build ./cmd/ovcpr
go test -race ./...
go vet ./...
```

Publishers generate submissions with:

```sh
dspm repo create --file release.dspk --repository ./ovcpr --download-url https://example.org/releases/1/release.dspk
ovcpr validate --repository ./ovcpr --baseline ./protected-baseline --network-policy policy/download-policy.json --cache-dir ./temporary-downloads --json
```

The validator checks the complete candidate graph and historical immutability,
then actually downloads and verifies **every new version**. Existing versions,
including deprecated versions, are never downloaded again by submission CI or
Pages builds. `structure` and `validate --offline` do not provide publication
approval. The final JSON report and process exit status both determine success.

See [contribution instructions](docs/contributing.md),
[format](docs/repository-format.md), and [operations](docs/operations.md).

The initial module lock uses an exact public-library commit hosted in
`CrSjimo/ovcpr-test` for integration testing. This is a Go module dependency with
go.sum verification, not copied source or an import of dspm internals. Before
production, replace that lock with the corresponding released dspm module,
release matching validator/indexer binaries, and update `policy/toolchain.json`
in a maintenance PR. All tool downloads are verified against SHA-256.
