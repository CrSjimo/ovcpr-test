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
	var metadataBytes, assetBytes, totalBytes int64
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
			return fail("INVALID_REPOSITORY_LAYOUT", p, fmt.Errorf("links are forbidden"))
		}
		parts := strings.Split(p, "/")
		for i := range parts {
			prefix := strings.Join(parts[:i+1], "/")
			key := strings.ToLower(prefix)
			if prev, ok := folded[key]; ok && prev != prefix {
				return fail("INVALID_REPOSITORY_LAYOUT", p, fmt.Errorf("case collision with %s", prev))
			}
			folded[key] = prefix
		}
		special := -1
		for i, part := range parts {
			if strings.HasPrefix(part, "@") {
				special = i
				break
			}
		}
		if special < 0 {
			if !d.IsDir() {
				return fail("INVALID_REPOSITORY_LAYOUT", p, fmt.Errorf("stray file outside reserved directories"))
			}
			if len(parts) > 1 {
				id := strings.Join(parts[1:], "/")
				if !packageinfo.IsValidPackageIdentifier(id) || len(id) > 128 || len(parts) > 9 {
					return fail("INVALID_REPOSITORY_LAYOUT", p, fmt.Errorf("invalid ID prefix"))
				}
				prefixes[id] = true
			}
			return nil
		}
		id := strings.Join(parts[1:special], "/")
		kind := parts[special]
		if id == "" || (kind != "@versions" && kind != "@assets") {
			return fail("INVALID_REPOSITORY_LAYOUT", p, fmt.Errorf("invalid reserved path"))
		}
		if kind == "@versions" && len(parts) > special+2 {
			return fail("INVALID_REPOSITORY_LAYOUT", p, fmt.Errorf("nested versions path"))
		}
		if d.IsDir() {
			if kind == "@versions" && len(parts) > special+1 {
				return fail("INVALID_REPOSITORY_LAYOUT", p, fmt.Errorf("versions subdirectory"))
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
			return fail("INVALID_REPOSITORY_LAYOUT", p, fmt.Errorf("special file"))
		}
		if err := checkHardlink(r, p); err != nil {
			return fail("INVALID_REPOSITORY_LAYOUT", p, err)
		}
		if st.Size() > 8<<20 {
			return fail("RESOURCE_LIMIT_EXCEEDED", p, fmt.Errorf("file too large"))
		}
		totalBytes += st.Size()
		if totalBytes > 800<<20 {
			return fail("RESOURCE_LIMIT_EXCEEDED", p, fmt.Errorf("site budget exceeded"))
		}
		b, e := r.ReadFile(p)
		if e != nil {
			return e
		}
		if bytes.HasPrefix(b, []byte("version https://git-lfs.github.com/spec/v1")) {
			return fail("INVALID_REPOSITORY_LAYOUT", p, fmt.Errorf("LFS pointer forbidden"))
		}
		out.Files[p] = b
		if kind == "@assets" {
			assetBytes += st.Size()
			assetTotals[id] += st.Size()
			if assetTotals[id] > 32<<20 {
				return fail("RESOURCE_LIMIT_EXCEEDED", p, fmt.Errorf("package asset budget exceeded"))
			}
			return nil
		}
		metadataBytes += st.Size()
		if metadataBytes > 64<<20 || len(out.Versions) >= 10000 {
			return fail("RESOURCE_LIMIT_EXCEEDED", p, fmt.Errorf("manifest budget exceeded"))
		}
		if path.Base(p) == "index.json" || !strings.HasSuffix(p, ".json") {
			return fail("INVALID_REPOSITORY_LAYOUT", p, fmt.Errorf("source index or non-manifest in versions"))
		}
		m, e := repo.DecodeManifest(b, repo.ManifestDecodeOptions{})
		if e != nil {
			return fail("INVALID_MANIFEST", p, e)
		}
		if m.ID != id || path.Base(p) != m.Version.String()+".json" {
			return fail("INVALID_MANIFEST", p, fmt.Errorf("directory identity or filename differs"))
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
	_ = assetBytes
	for id := range assetOwners {
		if !packages[id] {
			return out, fail("INVALID_REPOSITORY_LAYOUT", id, fmt.Errorf("assets without package"))
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
			return out, fail("INVALID_REPOSITORY_LAYOUT", prefix, fmt.Errorf("empty ID prefix"))
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
					return out, fail("INVALID_MANIFEST", v.Path, fmt.Errorf("missing exact asset %s", p))
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
				return fail("IMMUTABLE_VERSION_CHANGED", p, fmt.Errorf("historical asset changed or removed"))
			}
		}
	}
	for key, old := range baseline.Versions {
		next, ok := candidate.Versions[key]
		if !ok {
			return fail("IMMUTABLE_VERSION_CHANGED", old.Path, fmt.Errorf("historical version removed"))
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
			return fail("IMMUTABLE_VERSION_CHANGED", old.Path, fmt.Errorf("immutable publication fields changed"))
		}
		if old.Manifest.Deprecated == next.Manifest.Deprecated && old.Manifest.DeprecationReason == next.Manifest.DeprecationReason {
			return fail("IMMUTABLE_VERSION_CHANGED", old.Path, fmt.Errorf("format-only rewrite without state change"))
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
