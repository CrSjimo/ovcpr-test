"""Trusted CI coordinator. Execute this file from the protected baseline only."""
import argparse
import hashlib
import json
import os
from pathlib import Path
import subprocess
import tempfile
import urllib.request


def tool(trusted, work):
    lock = json.loads((trusted / "policy/toolchain.json").read_bytes())
    spec = lock["platforms"]["linux-amd64"]["validator"]
    assert spec == lock["platforms"]["linux-amd64"]["indexer"]
    assert spec["url"].startswith("https://")
    binary = work / "ovcpr"
    with urllib.request.urlopen(spec["url"], timeout=60) as response:
        assert response.url.startswith("https://")
        raw = response.read(64 * 1024 * 1024 + 1)
    assert len(raw) <= 64 * 1024 * 1024
    assert hashlib.sha256(raw).hexdigest() == spec["sha256"], "tool digest mismatch"
    binary.write_bytes(raw)
    binary.chmod(0o700)
    return binary


def invoke(binary, command, *args):
    result = subprocess.run([str(binary), command, *map(str, args), "--json"],
                            capture_output=True, text=True, check=False)
    if result.stderr:
        # Untrusted data is emitted as JSON, never as an Actions annotation.
        print(json.dumps({"toolStderr": result.stderr}), flush=True)
    report = json.loads(result.stdout)
    print(json.dumps(report, ensure_ascii=True), flush=True)
    assert result.returncode == 0 and report["ok"] is True
    assert report["protocolVersion"] == 1 and report["command"] == command
    return report["data"]


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("--stage", choices=["structure", "validate", "build", "pages"], required=True)
    parser.add_argument("--repository", type=Path, required=True)
    parser.add_argument("--baseline-sha")
    parser.add_argument("--candidate-sha", required=True)
    parser.add_argument("--output", type=Path)
    args = parser.parse_args()
    trusted = Path(__file__).resolve().parent.parent
    with tempfile.TemporaryDirectory(prefix="ovcpr-ci-", dir=os.environ["RUNNER_TEMP"]) as temporary:
        work = Path(temporary)
        binary = tool(trusted, work)
        candidate = work / "candidate"
        invoke(binary, "export", "--repository", args.repository, "--revision", args.candidate_sha,
               "--output", candidate)
        baseline_args = []
        if args.stage != "pages":
            assert args.baseline_sha
            baseline = work / "baseline"
            invoke(binary, "export", "--repository", args.repository, "--revision", args.baseline_sha,
                   "--output", baseline)
            invoke(binary, "classify", "--repository", args.repository,
                   "--baseline-revision", args.baseline_sha, "--candidate-revision", args.candidate_sha)
            baseline_args = ["--baseline", baseline, "--baseline-revision", args.baseline_sha]
        if args.stage in ("structure", "validate", "build"):
            command = "validate" if args.stage == "validate" else "structure"
            report = invoke(binary, command, "--repository", candidate, *baseline_args,
                            "--candidate-revision", args.candidate_sha,
                            "--network-policy", trusted / "policy/download-policy.json",
                            "--cache-dir", work / "archives")
            assert report["baselineChecked"] is True
            assert report["repositoryRevision"] == args.candidate_sha
            assert report["baselineRevision"] == args.baseline_sha
            if command == "validate":
                assert report["scope"] == "new-versions"
                assert report["requiredArchives"] == report["verifiedArchives"] == len(report["newVersions"])
            else:
                assert report["scope"] == "offline"
        if args.stage in ("build", "pages"):
            assert args.output
            invoke(binary, "build", "--repository", candidate, "--revision", args.candidate_sha,
                   "--output", args.output)
            metadata = {"eventSha": os.environ.get("GITHUB_SHA"), "targetSha": args.candidate_sha,
                        "tool": invoke(binary, "version")}
            (args.output / "build-info.json").write_text(json.dumps(metadata, indent=2) + "\n")
        summary = os.environ.get("GITHUB_STEP_SUMMARY")
        if summary:
            with open(summary, "a", encoding="utf-8") as output:
                output.write("```json\n" + json.dumps({"stage": args.stage, "candidateSha": args.candidate_sha,
                             "baselineSha": args.baseline_sha, "eventSha": os.environ.get("GITHUB_SHA")}, indent=2) + "\n```\n")


if __name__ == "__main__":
    main()
