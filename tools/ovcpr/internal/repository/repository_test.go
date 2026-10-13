// SPDX-License-Identifier: GPL-3.0-or-later

package repository

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"github.com/diffscope/diffscope-package-manager/packagearchive"
	repo "github.com/diffscope/diffscope-package-manager/packagerepository"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func artifact(t *testing.T, id, version string) []byte {
	t.Helper()
	var out bytes.Buffer
	z := zip.NewWriter(&out)
	manifest := map[string]any{"$version": "1", "runtimeLevel": 1, "id": id, "version": version, "license": "${root}/LICENSE"}
	raw, _ := json.Marshal(manifest)
	for p, b := range map[string][]byte{"desc.json": raw, "LICENSE": []byte("license\r\n\ufeff")} {
		w, err := z.Create(p)
		if err != nil {
			t.Fatal(err)
		}
		w.Write(b)
	}
	if err := z.Close(); err != nil {
		t.Fatal(err)
	}
	return out.Bytes()
}
func publish(t *testing.T, root string, raw []byte, downloadURL string) Version {
	t.Helper()
	a, err := packagearchive.NewReaderAt(bytes.NewReader(raw), int64(len(raw)), packagearchive.DefaultLimits())
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	p, err := repo.ProjectFS(context.Background(), a.Files(), repo.ProjectionOptions{})
	if err != nil {
		t.Fatal(err)
	}
	m, assets, err := repo.BuildManifest(p, repo.Publication{DownloadURL: downloadURL, DownloadSha512: repo.Sha512(raw), DownloadSize: int64(len(raw))}, nil)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := repo.MarshalManifest(m)
	target := "packages/" + m.ID + "/@versions/" + m.Version.String() + ".json"
	write := func(p string, b []byte) {
		dst := filepath.Join(root, filepath.FromSlash(p))
		os.MkdirAll(filepath.Dir(dst), 0755)
		if err := os.WriteFile(dst, b, 0644); err != nil {
			t.Fatal(err)
		}
	}
	write(target, b)
	for _, asset := range assets {
		b, _ := a.Files().Open(asset.PackagePath)
		var data bytes.Buffer
		data.ReadFrom(b)
		b.Close()
		write("packages/"+m.ID+"/@assets/"+asset.RepositoryPath, data.Bytes())
	}
	return Version{Manifest: m, Raw: b, Path: target}
}
func TestValidateActualGETAndHistoricalStateOnly(t *testing.T) {
	ctx := context.Background()
	raw := artifact(t, "foo", "1")
	gets := 0
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { gets++; w.Write(raw) }))
	defer server.Close()
	root := t.TempDir()
	v := publish(t, root, raw, server.URL+"/archive")
	u, _ := url.Parse(server.URL)
	policy := DefaultPolicy()
	policy.AllowedPorts = []string{u.Port()}
	result, err := Validate(ctx, ValidationOptions{Repository: root, Policy: policy, client: server.Client()})
	if err != nil || result.VerifiedArchives != 1 || result.VerifiedAssets != 1 || gets != 1 {
		t.Fatal(result, err, gets)
	}
	base := t.TempDir()
	publish(t, base, raw, server.URL+"/archive")
	m := v.Manifest
	m.Deprecated = true
	m.DeprecationReason = "test"
	b, _ := repo.MarshalManifest(m)
	os.WriteFile(filepath.Join(root, filepath.FromSlash(v.Path)), b, 0644)
	result, err = Validate(ctx, ValidationOptions{Repository: root, Baseline: base, Policy: policy})
	if err != nil || result.RequiredArchives != 0 || gets != 1 {
		t.Fatal(result, err, gets)
	}
	m.DownloadURL = "https://example.org/new"
	b, _ = repo.MarshalManifest(m)
	os.WriteFile(filepath.Join(root, filepath.FromSlash(v.Path)), b, 0644)
	if _, err = Validate(ctx, ValidationOptions{Repository: root, Baseline: base, Policy: policy}); err == nil {
		t.Fatal("immutable URL changed")
	}
}

func TestSingerIntegerValidationAndBuild(t *testing.T) {
	var archive bytes.Buffer
	z := zip.NewWriter(&archive)
	for p, contents := range map[string]string{
		"desc.json":   `{"$version":"1","runtimeLevel":1,"id":"voice","version":"1","contributions":{"singer":[{"id":"main","path":"singer.json"}]}}`,
		"singer.json": `{"interface":"example.Singer","level":2,"variant":"v","name":"Voice"}`,
	} {
		w, err := z.Create(p)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = w.Write([]byte(contents)); err != nil {
			t.Fatal(err)
		}
	}
	if err := z.Close(); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write(archive.Bytes()) }))
	defer server.Close()
	root := t.TempDir()
	v := publish(t, root, archive.Bytes(), server.URL+"/archive")
	u, _ := url.Parse(server.URL)
	policy := DefaultPolicy()
	policy.AllowedPorts = []string{u.Port()}
	if result, err := Validate(context.Background(), ValidationOptions{Repository: root, Policy: policy, client: server.Client()}); err != nil || result.VerifiedArchives != 1 {
		t.Fatal(result, err)
	}
	output := filepath.Join(t.TempDir(), "site")
	if _, err := Build(context.Background(), root, output, strings.Repeat("1", 40)); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(filepath.Join(output, filepath.FromSlash(v.Path)))
	if err != nil {
		t.Fatal(err)
	}
	m, err := repo.DecodeManifest(raw, repo.ManifestDecodeOptions{})
	if err != nil || len(m.Singers) != 1 || m.Singers[0].Level != 2 || !bytes.Contains(raw, []byte(`"level": 2`)) {
		t.Fatal(string(raw), err)
	}
	if !bytes.Equal(raw, v.Raw) {
		t.Fatal("Site generation changed source manifest bytes.")
	}
}
func TestPrefixPackagesBuildDeterminismAndOrder(t *testing.T) {
	root := t.TempDir()
	publish(t, root, artifact(t, "foo", "2"), "https://example.org/2")
	publish(t, root, artifact(t, "foo", "10"), "https://example.org/10")
	publish(t, root, artifact(t, "foo/bar", "1"), "https://example.org/bar")
	revision := strings.Repeat("1", 40)
	a := filepath.Join(t.TempDir(), "a")
	b := filepath.Join(t.TempDir(), "b")
	result, err := Build(context.Background(), root, a, revision)
	if err != nil || result.Packages != 2 {
		t.Fatal(result, err)
	}
	if _, err = Build(context.Background(), root, b, revision); err != nil {
		t.Fatal(err)
	}
	pa, _ := os.ReadFile(filepath.Join(a, "packages", "foo", "@versions", "index.json"))
	pb, _ := os.ReadFile(filepath.Join(b, "packages", "foo", "@versions", "index.json"))
	if !bytes.Equal(pa, pb) {
		t.Fatal("nondeterministic index")
	}
	index, err := repo.DecodeVersionsIndex(pa, repo.DecodeLimits{})
	if err != nil || index.LatestVersion.String() != "10.0.0.0" {
		t.Fatal(index, err)
	}
	rootRaw, _ := os.ReadFile(filepath.Join(a, "packages", "index.json"))
	ri, err := repo.DecodePackagesIndex(rootRaw, repo.DecodeLimits{})
	if err != nil || ri.Packages[0].VersionsIndexSha512 != repo.Sha512(pa) {
		t.Fatal(ri, err)
	}
}
func TestEmptyAndInvalidLayout(t *testing.T) {
	root := t.TempDir()
	if result, err := Build(context.Background(), root, filepath.Join(t.TempDir(), "site"), strings.Repeat("0", 40)); err != nil || result.Packages != 0 {
		t.Fatal(result, err)
	}
	os.MkdirAll(filepath.Join(root, "packages", "CON"), 0755)
	if _, err := Scan(context.Background(), root); err == nil {
		t.Fatal("reserved/empty prefix accepted")
	}
}
func TestGitExportUsesBlobBytes(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	publish(t, root, artifact(t, "foo", "1"), "https://example.org/f")
	if _, err := git(ctx, root, "init", "-b", "main"); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(root, ".gitattributes"), []byte("packages/** text eol=crlf\n"), 0644)
	if _, err := git(ctx, root, "add", "."); err != nil {
		t.Fatal(err)
	}
	if _, err := git(ctx, root, "-c", "user.name=Test", "-c", "user.email=test@example.org", "commit", "-m", "fixture"); err != nil {
		t.Fatal(err)
	}
	shaRaw, _ := git(ctx, root, "rev-parse", "HEAD")
	sha := strings.TrimSpace(string(shaRaw))
	out := filepath.Join(t.TempDir(), "data")
	if err := Export(ctx, root, sha, out); err != nil {
		t.Fatal(err)
	}
	blob, err := git(ctx, root, "show", sha+":packages/foo/@assets/1.0.0.0/LICENSE")
	if err != nil {
		t.Fatal(err)
	}
	actual, _ := os.ReadFile(filepath.Join(out, "packages", "foo", "@assets", "1.0.0.0", "LICENSE"))
	if !bytes.Equal(blob, actual) {
		t.Fatal("export changed Git bytes")
	}
}

func TestValidationRejectsArchiveSizeMismatch(t *testing.T) {
	raw := artifact(t, "voice", "1")
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write(raw) }))
	defer server.Close()
	root := t.TempDir()
	v := publish(t, root, raw, server.URL+"/archive")
	v.Manifest.DownloadSize++
	body, err := repo.MarshalManifest(v.Manifest)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(root, filepath.FromSlash(v.Path)), body, 0644); err != nil {
		t.Fatal(err)
	}
	u, _ := url.Parse(server.URL)
	policy := DefaultPolicy()
	policy.AllowedPorts = []string{u.Port()}
	if _, err = Validate(context.Background(), ValidationOptions{Repository: root, Policy: policy, client: server.Client()}); err == nil || !strings.Contains(err.Error(), "archive size") {
		t.Fatal(err)
	}
}
