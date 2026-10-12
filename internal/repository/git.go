package repository

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	repo "github.com/diffscope/diffscope-package-manager/packagerepository"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

func git(ctx context.Context, root string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, "git", append([]string{"-C", root}, args...)...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	b, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("git failed: %s: %w", stderr.String(), err)
	}
	return b, nil
}
func Commit(ctx context.Context, root, revision string) (string, error) {
	if len(revision) != 40 && len(revision) != 64 {
		return "", fmt.Errorf("full commit SHA required")
	}
	for _, c := range revision {
		if !strings.ContainsRune("0123456789abcdef", c) {
			return "", fmt.Errorf("invalid SHA")
		}
	}
	b, err := git(ctx, root, "rev-parse", "--verify", revision+"^{commit}")
	return strings.TrimSpace(string(b)), err
}

// Export reads Git blobs without checkout filters, archive attributes or code
// execution from the candidate. The caller supplies a fixed commit SHA.
func Export(ctx context.Context, root, revision, output string) error {
	sha, err := Commit(ctx, root, revision)
	if err != nil {
		return err
	}
	listing, err := git(ctx, root, "ls-tree", "-rz", "--full-tree", sha)
	if err != nil {
		return err
	}
	if _, err = os.Stat(output); err == nil {
		return fmt.Errorf("export output already exists")
	}
	if err = os.MkdirAll(output, 0700); err != nil {
		return err
	}
	r, err := os.OpenRoot(output)
	if err != nil {
		return err
	}
	defer r.Close()
	cmd := exec.CommandContext(ctx, "git", "-C", root, "cat-file", "--batch")
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	if err = cmd.Start(); err != nil {
		return err
	}
	finished := false
	defer func() {
		stdin.Close()
		if !finished {
			cmd.Process.Kill()
			cmd.Wait()
		}
	}()
	reader := bufio.NewReader(stdout)
	var total int64
	for _, record := range bytes.Split(listing, []byte{0}) {
		if len(record) == 0 {
			continue
		}
		header, p, ok := bytes.Cut(record, []byte{'\t'})
		if !ok {
			return fmt.Errorf("invalid Git tree")
		}
		parts := strings.Fields(string(header))
		if len(parts) != 3 {
			return fmt.Errorf("invalid Git tree header")
		}
		name := string(p)
		if strings.HasPrefix(name, ".dspm-repo/") {
			return fail("INVALID_REPOSITORY_LAYOUT", name, fmt.Errorf("committed local generator state"))
		}
		if name != "packages" && !strings.HasPrefix(name, "packages/") {
			continue
		}
		if parts[0] != "100644" && parts[0] != "100755" || parts[1] != "blob" {
			return fail("INVALID_REPOSITORY_LAYOUT", name, fmt.Errorf("nonregular Git object"))
		}
		if err = repo.PortablePath(name); err != nil {
			return err
		}
		if _, err = io.WriteString(stdin, parts[2]+"\n"); err != nil {
			return err
		}
		line, err := reader.ReadString('\n')
		if err != nil {
			return err
		}
		fields := strings.Fields(line)
		if len(fields) != 3 || fields[0] != parts[2] || fields[1] != "blob" {
			return fmt.Errorf("unexpected Git blob header")
		}
		size, err := strconv.ParseInt(fields[2], 10, 64)
		if err != nil || size < 0 || size > 8<<20 || total+size > 800<<20 {
			return fail("RESOURCE_LIMIT_EXCEEDED", name, fmt.Errorf("Git data exceeds budget"))
		}
		total += size
		if err = r.MkdirAll(filepath.Dir(name), 0700); err != nil {
			return err
		}
		f, err := r.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
		if err != nil {
			return err
		}
		_, err = io.CopyN(f, reader, size)
		e := f.Close()
		if err != nil {
			return err
		}
		if e != nil {
			return e
		}
		newline, err := reader.ReadByte()
		if err != nil || newline != '\n' {
			return fmt.Errorf("invalid Git blob separator")
		}
	}
	stdin.Close()
	err = cmd.Wait()
	finished = true
	return err
}
func Classify(ctx context.Context, root, baseline, candidate string) (map[string]any, error) {
	if _, err := Commit(ctx, root, baseline); err != nil {
		return nil, err
	}
	if _, err := Commit(ctx, root, candidate); err != nil {
		return nil, err
	}
	b, err := git(ctx, root, "diff", "--name-only", "-z", baseline, candidate, "--")
	if err != nil {
		return nil, err
	}
	packages, control := false, false
	paths := []string{}
	for _, p := range bytes.Split(b, []byte{0}) {
		if len(p) == 0 {
			continue
		}
		name := string(p)
		paths = append(paths, name)
		if strings.HasPrefix(name, "packages/") {
			packages = true
		}
		for _, prefix := range []string{"tools/", "schemas/", "policy/", ".github/", "cmd/", "internal/"} {
			if strings.HasPrefix(name, prefix) {
				control = true
			}
		}
		for _, file := range []string{"go.mod", "go.sum", ".gitattributes", ".gitignore"} {
			if name == file {
				control = true
			}
		}
	}
	if packages && control {
		return nil, fail("MIXED_CONTROL_AND_PACKAGE_CHANGE", "", fmt.Errorf("package data and control plane must use separate PRs"))
	}
	return map[string]any{"packageChanges": packages, "controlChanges": control, "paths": paths}, nil
}
