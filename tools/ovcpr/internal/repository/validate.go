package repository

import (
	"context"
	"fmt"
	"github.com/diffscope/diffscope-package-manager/packagearchive"
	dep "github.com/diffscope/diffscope-package-manager/packagedependency"
	"github.com/diffscope/diffscope-package-manager/packageinfo"
	repo "github.com/diffscope/diffscope-package-manager/packagerepository"
	"io/fs"
	"net/http"
	"os"
	"path"
	"runtime/debug"
)

var ToolVersion = "0.1.0-dev"

type ValidationOptions struct {
	client                                                              *http.Client // injected only by in-package transport tests
	Repository, Baseline, CandidateRevision, BaselineRevision, CacheDir string
	Policy                                                              NetworkPolicy
	Offline                                                             bool
}
type ArchiveResult struct {
	packageinfo.Identity
	DownloadSha512    string `json:"downloadSha512"`
	ContentTreeSha512 string `json:"contentTreeSha512"`
	URLVerified       bool   `json:"urlVerified"`
}
type Validation struct {
	Scope              string                 `json:"scope"`
	RepositoryRevision string                 `json:"repositoryRevision,omitempty"`
	BaselineRevision   string                 `json:"baselineRevision,omitempty"`
	BaselineChecked    bool                   `json:"baselineChecked"`
	ToolVersion        string                 `json:"toolVersion"`
	DSPMVersion        string                 `json:"dspmVersion"`
	PolicyVersion      int                    `json:"policyVersion"`
	CheckedManifests   int                    `json:"checkedManifests"`
	RequiredArchives   int                    `json:"requiredArchives"`
	VerifiedArchives   int                    `json:"verifiedArchives"`
	HistoricalVersions int                    `json:"historicalVersions"`
	VerifiedAssets     int                    `json:"verifiedAssets"`
	Resolved           []dep.Edge             `json:"resolved"`
	NewVersions        []packageinfo.Identity `json:"newVersions"`
	Archives           []ArchiveResult        `json:"archives"`
}

func LibraryVersion() string {
	if info, ok := debug.ReadBuildInfo(); ok {
		for _, d := range info.Deps {
			if d.Path == "github.com/diffscope/diffscope-package-manager" {
				if d.Replace != nil {
					return d.Path + "@" + d.Version + " => " + d.Replace.Path + "@" + d.Replace.Version
				}
				return d.Path + "@" + d.Version
			}
		}
	}
	return "unknown"
}
func Validate(ctx context.Context, o ValidationOptions) (Validation, error) {
	out := Validation{Scope: "new-versions", RepositoryRevision: o.CandidateRevision, BaselineRevision: o.BaselineRevision, BaselineChecked: o.Baseline != "", ToolVersion: ToolVersion, DSPMVersion: LibraryVersion(), PolicyVersion: o.Policy.PolicyVersion, NewVersions: []packageinfo.Identity{}, Archives: []ArchiveResult{}}
	tree, err := Scan(ctx, o.Repository)
	if err != nil {
		return out, err
	}
	baseline := Tree{Versions: map[string]Version{}, Files: map[string][]byte{}}
	if o.Baseline != "" {
		baseline, err = Scan(ctx, o.Baseline)
		if err != nil {
			return out, err
		}
		if err = Immutability(tree, baseline); err != nil {
			return out, err
		}
	}
	newVersions := NewVersions(tree, baseline)
	out.CheckedManifests = len(tree.Versions)
	out.RequiredArchives = len(newVersions)
	out.HistoricalVersions = len(tree.Versions) - len(newVersions)
	out.Resolved = tree.Graph.Edges
	for _, v := range newVersions {
		out.NewVersions = append(out.NewVersions, v.Manifest.Identity)
	}
	if o.Offline {
		out.Scope = "offline"
		return out, nil
	}
	tmp := o.CacheDir
	if tmp == "" {
		tmp, err = os.MkdirTemp("", "ovcpr-validation-")
		if err != nil {
			return out, err
		}
		defer os.RemoveAll(tmp)
	} else {
		if err = os.MkdirAll(tmp, 0700); err != nil {
			return out, err
		}
	}
	client := o.Policy.Client()
	if o.client != nil {
		client = o.client
	}
	defer client.CloseIdleConnections()
	r, err := os.OpenRoot(o.Repository)
	if err != nil {
		return out, err
	}
	defer r.Close()
	for _, v := range newVersions {
		m := v.Manifest
		f, digest, err := o.Policy.Download(ctx, client, m.DownloadURL, tmp)
		if err != nil {
			return out, err
		}
		validate := func() error {
			defer func() { name := f.Name(); f.Close(); os.Remove(name) }()
			if digest != m.DownloadSha512 {
				return &Error{Code: "DOWNLOAD_HASH_MISMATCH", Message: "archive file digest differs", Path: v.Path, Details: map[string]string{"expected": m.DownloadSha512, "actual": digest}}
			}
			st, err := f.Stat()
			if err != nil {
				return err
			}
			limits := packagearchive.DefaultLimits()
			limits.FileBytes = o.Policy.MaxFileBytes
			limits.TotalBytes = o.Policy.MaxExpandedBytes
			a, err := packagearchive.NewReaderAt(f, st.Size(), limits)
			if err != nil {
				return fail("INVALID_PACKAGE_ARCHIVE", v.Path, err)
			}
			defer a.Close()
			treeHash, err := a.Hash(ctx)
			if err != nil {
				return fail("INVALID_PACKAGE_ARCHIVE", v.Path, err)
			}
			projection, err := repo.ProjectFS(ctx, a.Files(), repo.ProjectionOptions{Limits: repo.ProjectionLimits{MaxAssetBytes: 32 << 20}})
			if err != nil {
				return fail("INVALID_PACKAGE_ARCHIVE", v.Path, err)
			}
			diff, err := repo.CompareMetadata(m, projection)
			if err != nil {
				return err
			}
			if len(diff) > 0 {
				return &Error{Code: "PACKAGE_METADATA_MISMATCH", Path: v.Path, Message: "archive metadata differs", Details: diff}
			}
			mapping, err := repo.ResourceMappings(m, projection)
			if err != nil {
				return err
			}
			assets, err := fs.Sub(r.FS(), path.Join("packages", m.ID, "@assets"))
			if err != nil {
				return err
			}
			diff, err = repo.CompareAssetBytes(ctx, mapping, a.Files(), assets)
			if err != nil {
				return err
			}
			if len(diff) > 0 {
				return &Error{Code: "ASSET_CONTENT_MISMATCH", Path: v.Path, Message: "asset bytes differ", Details: diff}
			}
			out.VerifiedAssets += len(mapping)
			out.VerifiedArchives++
			out.Archives = append(out.Archives, ArchiveResult{m.Identity, digest, treeHash, true})
			return nil
		}
		if err = validate(); err != nil {
			return out, err
		}
	}
	if out.VerifiedArchives != out.RequiredArchives {
		return out, fmt.Errorf("incomplete validation")
	}
	return out, nil
}
