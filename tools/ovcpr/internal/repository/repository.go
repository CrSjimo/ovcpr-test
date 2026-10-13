// SPDX-License-Identifier: GPL-3.0-or-later

// Package repository implements the repository-specific v1 acceptance rules.
package repository

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	dep "github.com/diffscope/diffscope-package-manager/packagedependency"
	"github.com/diffscope/diffscope-package-manager/packageinfo"
	repo "github.com/diffscope/diffscope-package-manager/packagerepository"
	"io/fs"
	"os"
	"path"
	"reflect"
	"sort"
	"strings"
)

type Error struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Path    string `json:"path,omitempty"`
	Details any    `json:"details,omitempty"`
}

func (e *Error) Error() string { return e.Code + ": " + e.Message }
func fail(code, p string, err error) error {
	result := &Error{Code: code, Path: p, Message: err.Error()}
	var graph *dep.Error
	if errors.As(err, &graph) {
		result.Details = map[string]any{"kind": graph.Kind, "root": graph.Root, "requester": graph.Requester, "required": graph.Required, "chain": graph.Chain}
	}
	return result
}

type Version struct {
	Manifest repo.RemoteManifest
	Raw      []byte
	Path     string
}
type Tree struct {
	Versions map[string]Version
	Files    map[string][]byte
	Graph    dep.Graph
}

func Scan(ctx context.Context, root string) (Tree, error) {
	out := Tree{Versions: map[string]Version{}, Files: map[string][]byte{}}
	r, err := os.OpenRoot(root)
	if err != nil {
		return out, err
	}
	defer r.Close()
	folded := map[string]string{}
	packages := map[string]bool{}
	assetOwners := map[string]bool{}
	var metadataBytes, totalBytes int64
	assetTotals := map[string]int64{}
	prefixes := map[string]bool{}
	err = fs.WalkDir(r.FS(), "packages", func(p string, d fs.DirEntry, e error) error {
		if os.IsNotExist(e) && p == "packages" {
			return nil
		}
		if e != nil {
			return e
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := repo.PortablePath(p); err != nil {
			return fail("INVALID_REPOSITORY_LAYOUT", p, err)
		}
		if d.Type()&os.ModeSymlink != 0 {
			return fail("INVALID_REPOSITORY_LAYOUT", p, fmt.Errorf("Symbolic links are forbidden."))
		}
		parts := strings.Split(p, "/")
		key := strings.ToLower(p)
		if prev, ok := folded[key]; ok && prev != p {
			return fail("INVALID_REPOSITORY_LAYOUT", p, fmt.Errorf("The path has a case collision with %s.", prev))
		}
		folded[key] = p
		special := -1
		for i, part := range parts {
			if strings.HasPrefix(part, "@") {
				special = i
				break
			}
		}
		if special < 0 {
			if !d.IsDir() {
				return fail("INVALID_REPOSITORY_LAYOUT", p, fmt.Errorf("A file is outside the permitted reserved directories."))
			}
			if len(parts) > 1 {
				id := strings.Join(parts[1:], "/")
				if !packageinfo.IsValidPackageIdentifier(id) || len(id) > 128 || len(parts) > 9 {
					return fail("INVALID_REPOSITORY_LAYOUT", p, fmt.Errorf("The package ID prefix is invalid."))
				}
				prefixes[id] = true
			}
			return nil
		}
		id := strings.Join(parts[1:special], "/")
		kind := parts[special]
		if id == "" || (kind != "@versions" && kind != "@assets") {
			return fail("INVALID_REPOSITORY_LAYOUT", p, fmt.Errorf("The reserved path is invalid."))
		}
		if kind == "@versions" && len(parts) > special+2 {
			return fail("INVALID_REPOSITORY_LAYOUT", p, fmt.Errorf("The versions path has invalid nesting."))
		}
		if d.IsDir() {
			if kind == "@versions" && len(parts) > special+1 {
				return fail("INVALID_REPOSITORY_LAYOUT", p, fmt.Errorf("Subdirectories are forbidden inside @versions."))
			}
			if kind == "@assets" {
				assetOwners[id] = true
			}
			return nil
		}
		st, e := d.Info()
		if e != nil {
			return e
		}
		if !st.Mode().IsRegular() {
			return fail("INVALID_REPOSITORY_LAYOUT", p, fmt.Errorf("Special files are forbidden."))
		}
		if err := checkHardlink(r, p); err != nil {
			return fail("INVALID_REPOSITORY_LAYOUT", p, err)
		}
		if st.Size() > 8<<20 {
			return fail("RESOURCE_LIMIT_EXCEEDED", p, fmt.Errorf("The file exceeds its byte limit."))
		}
		totalBytes += st.Size()
		if totalBytes > 800<<20 {
			return fail("RESOURCE_LIMIT_EXCEEDED", p, fmt.Errorf("The site exceeds its byte budget."))
		}
		b, e := r.ReadFile(p)
		if e != nil {
			return e
		}
		if bytes.HasPrefix(b, []byte("version https://git-lfs.github.com/spec/v1")) {
			return fail("INVALID_REPOSITORY_LAYOUT", p, fmt.Errorf("LFS pointers are forbidden."))
		}
		out.Files[p] = b
		if kind == "@assets" {
			assetTotals[id] += st.Size()
			if assetTotals[id] > 32<<20 {
				return fail("RESOURCE_LIMIT_EXCEEDED", p, fmt.Errorf("The package assets exceed their byte budget."))
			}
			return nil
		}
		metadataBytes += st.Size()
		if metadataBytes > 64<<20 || len(out.Versions) >= 10000 {
			return fail("RESOURCE_LIMIT_EXCEEDED", p, fmt.Errorf("The repository manifests exceed their byte budget."))
		}
		if path.Base(p) == "index.json" || !strings.HasSuffix(p, ".json") {
			return fail("INVALID_REPOSITORY_LAYOUT", p, fmt.Errorf("The source versions directory contains a generated index or non-manifest file."))
		}
		m, e := repo.DecodeManifest(b, repo.ManifestDecodeOptions{})
		if e != nil {
			return fail("INVALID_MANIFEST", p, e)
		}
		if m.ID != id || path.Base(p) != m.Version.String()+".json" {
			return fail("INVALID_MANIFEST", p, fmt.Errorf("The directory identity or filename differs from the manifest identity."))
		}
		if e = StableURL(m.DownloadURL); e != nil {
			return fail("DOWNLOAD_POLICY_VIOLATION", p, e)
		}
		out.Versions[m.String()] = Version{m, b, p}
		packages[id] = true
		return nil
	})
	if err != nil {
		return out, err
	}
	for id := range assetOwners {
		if !packages[id] {
			return out, fail("INVALID_REPOSITORY_LAYOUT", id, fmt.Errorf("Documentation assets require a corresponding package version."))
		}
	}
	for prefix := range prefixes {
		found := false
		for id := range packages {
			if id == prefix || strings.HasPrefix(id, prefix+"/") {
				found = true
				break
			}
		}
		if !found {
			return out, fail("INVALID_REPOSITORY_LAYOUT", prefix, fmt.Errorf("The package ID prefix contains no versions."))
		}
	}
	all := []dep.Candidate{}
	for _, v := range out.Versions {
		m := v.Manifest
		all = append(all, dep.Candidate{Identity: m.Identity, CompatVersion: m.CompatVersion, Dependencies: m.Dependencies})
		for _, text := range []packageinfo.Text{m.License, m.Readme} {
			for _, relative := range text {
				p := path.Join("packages", m.ID, "@assets", relative)
				if _, ok := out.Files[p]; !ok {
					return out, fail("INVALID_MANIFEST", v.Path, fmt.Errorf("The exact asset %s is missing.", p))
				}
			}
		}
	}
	out.Graph, err = dep.ValidateAll(ctx, all, dep.DefaultLimits())
	if err != nil {
		code := "UNRESOLVED_DEPENDENCY"
		if e, ok := err.(*dep.Error); ok && e.Kind == "DEPENDENCY_CYCLE" {
			code = "DEPENDENCY_CYCLE"
		}
		return out, fail(code, "", err)
	}
	return out, nil
}
func Immutability(candidate, baseline Tree) error {
	for p, b := range baseline.Files {
		if strings.Contains(p, "/@assets/") {
			actual, ok := candidate.Files[p]
			if !ok || !bytes.Equal(actual, b) {
				return fail("IMMUTABLE_VERSION_CHANGED", p, fmt.Errorf("An accepted asset was changed or removed."))
			}
		}
	}
	for key, old := range baseline.Versions {
		next, ok := candidate.Versions[key]
		if !ok {
			return fail("IMMUTABLE_VERSION_CHANGED", old.Path, fmt.Errorf("An accepted version was removed."))
		}
		if bytes.Equal(old.Raw, next.Raw) {
			continue
		}
		a, err := packageinfo.DecodeJSON(old.Raw)
		if err != nil {
			return err
		}
		b, err := packageinfo.DecodeJSON(next.Raw)
		if err != nil {
			return err
		}
		delete(a, "deprecated")
		delete(a, "deprecationReason")
		delete(b, "deprecated")
		delete(b, "deprecationReason")
		if !reflect.DeepEqual(a, b) {
			return fail("IMMUTABLE_VERSION_CHANGED", old.Path, fmt.Errorf("Immutable publication fields were changed."))
		}
		if old.Manifest.Deprecated == next.Manifest.Deprecated && old.Manifest.DeprecationReason == next.Manifest.DeprecationReason {
			return fail("IMMUTABLE_VERSION_CHANGED", old.Path, fmt.Errorf("The accepted manifest was reformatted without a deprecation change."))
		}
	}
	return nil
}
func NewVersions(candidate, baseline Tree) []Version {
	out := []Version{}
	for key, v := range candidate.Versions {
		if _, ok := baseline.Versions[key]; !ok {
			out = append(out, v)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Manifest.String() < out[j].Manifest.String() })
	return out
}
