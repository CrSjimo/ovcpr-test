// SPDX-License-Identifier: GPL-3.0-or-later

package repository

import (
	"context"
	"fmt"
	repo "github.com/diffscope/diffscope-package-manager/packagerepository"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type BuildResult struct {
	RepositoryRevision string `json:"repositoryRevision"`
	Packages           int    `json:"packages"`
	Versions           int    `json:"versions"`
	Files              int    `json:"files"`
	Bytes              int64  `json:"bytes"`
	Output             string `json:"output"`
}

func Build(ctx context.Context, root, output, revision string) (BuildResult, error) {
	result := BuildResult{RepositoryRevision: revision, Output: output}
	tree, err := Scan(ctx, root)
	if err != nil {
		return result, err
	}
	index := repo.PackagesIndex{RepositoryFormatVersion: 1, RepositoryRevision: revision, Packages: []repo.PackageEntry{}}
	byID := map[string][]Version{}
	for _, v := range tree.Versions {
		byID[v.Manifest.ID] = append(byID[v.Manifest.ID], v)
	}
	ids := []string{}
	for id := range byID {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	generated := map[string][]byte{}
	for _, id := range ids {
		versions := byID[id]
		sort.Slice(versions, func(i, j int) bool { return versions[i].Manifest.Version.Compare(versions[j].Manifest.Version) > 0 })
		vi := repo.VersionsIndex{RepositoryFormatVersion: 1, ID: id, LatestVersion: versions[0].Manifest.Version, Versions: []repo.VersionEntry{}}
		for _, v := range versions {
			m := v.Manifest
			vi.Versions = append(vi.Versions, repo.VersionEntry{Version: m.Version, CompatVersion: m.CompatVersion, ManifestSha512: repo.Sha512(v.Raw), Deprecated: m.Deprecated})
		}
		raw, err := repo.Marshal(vi)
		if err != nil {
			return result, err
		}
		if _, err = repo.DecodeVersionsIndex(raw, repo.DecodeLimits{MaxBytes: 8 << 20}); err != nil {
			return result, err
		}
		generated["packages/"+id+"/@versions/index.json"] = raw
		index.Packages = append(index.Packages, repo.PackageEntry{ID: id, LatestVersion: vi.LatestVersion, VersionsIndexSha512: repo.Sha512(raw)})
	}
	raw, err := repo.Marshal(index)
	if err != nil {
		return result, err
	}
	if _, err = repo.DecodePackagesIndex(raw, repo.DecodeLimits{MaxBytes: 16 << 20}); err != nil {
		return result, err
	}
	generated["packages/index.json"] = raw
	generated[".nojekyll"] = []byte{}
	generated["index.html"] = []byte("<!doctype html><html lang=\"en\"><meta charset=\"utf-8\"><meta name=\"viewport\" content=\"width=device-width\"><title>OVCPR</title><main><h1>DiffScope community package repository</h1><p>OVCPR v1 provides immutable package manifests and documentation. Archives are hosted by their publishers.</p><a href=\"packages/index.json\">Package index</a></main></html>\n")
	for p, b := range tree.Files {
		generated[p] = b
	}
	for _, b := range generated {
		result.Bytes += int64(len(b))
	}
	if result.Bytes > 800<<20 {
		return result, fail("RESOURCE_LIMIT_EXCEEDED", "", fmt.Errorf("The site exceeds its byte budget."))
	}
	result.Files = len(generated)
	result.Packages = len(ids)
	result.Versions = len(tree.Versions)
	absolute, err := filepath.Abs(output)
	if err != nil {
		return result, err
	}
	if filepath.Clean(absolute) == filepath.Clean(root) || strings.HasPrefix(filepath.Clean(root), absolute+string(filepath.Separator)) {
		return result, fmt.Errorf("The output directory overlaps the source directory.")
	}
	if _, err = os.Stat(absolute); err == nil {
		return result, fmt.Errorf("The output directory must not already exist.")
	}
	parent := filepath.Dir(absolute)
	if err = os.MkdirAll(parent, 0755); err != nil {
		return result, err
	}
	tmp, err := os.MkdirTemp(parent, ".ovcpr-site-")
	if err != nil {
		return result, err
	}
	defer os.RemoveAll(tmp)
	paths := []string{}
	for p := range generated {
		paths = append(paths, p)
	}
	sort.Strings(paths)
	for _, p := range paths {
		if err = ctx.Err(); err != nil {
			return result, err
		}
		dst := filepath.Join(tmp, filepath.FromSlash(p))
		if err = os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
			return result, err
		}
		if err = os.WriteFile(dst, generated[p], 0644); err != nil {
			return result, err
		}
	}
	if err = os.Rename(tmp, absolute); err != nil {
		return result, err
	}
	result.Output = absolute
	return result, nil
}
