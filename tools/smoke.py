# SPDX-License-Identifier: Apache-2.0

"""Verifies all linked metadata bytes anonymously after deployment."""
import argparse
import hashlib
import json
import time
import urllib.parse
import urllib.request


def get(base, relative):
    """Returns raw bytes and decoded JSON or raises an error for failed or oversized responses."""
    url = base.rstrip("/") + "/" + "/".join(urllib.parse.quote(p, safe="") for p in relative.split("/"))
    request = urllib.request.Request(url, headers={"Cache-Control": "no-cache", "Accept-Encoding": "identity"})
    with urllib.request.urlopen(request, timeout=30) as response:
        raw = response.read(16 * 1024 * 1024 + 1)
    assert len(raw) <= 16 * 1024 * 1024
    return raw, json.loads(raw)


def check(base, revision):
    """Returns a successful revision and digest report or raises an error for invalid metadata."""
    _, root = get(base, "packages/index.json")
    assert root["repositoryRevision"] == revision
    count = 0
    for package in root["packages"]:
        raw, versions = get(base, "packages/" + package["id"] + "/@versions/index.json")
        assert hashlib.sha512(raw).hexdigest() == package["versionsIndexSha512"]
        assert versions["id"] == package["id"] and versions["latestVersion"] == package["latestVersion"]
        for version in versions["versions"]:
            raw, manifest = get(base, "packages/" + package["id"] + "/@versions/" + version["version"] + ".json")
            assert hashlib.sha512(raw).hexdigest() == version["manifestSha512"]
            assert manifest["id"] == package["id"] and manifest["version"] == version["version"]
            assert manifest["compatVersion"] == version["compatVersion"]
            assert type(manifest["downloadSize"]) is int and manifest["downloadSize"] > 0
            assert manifest.get("deprecated", False) == version["deprecated"]
            count += 1
    return {"ok": True, "repositoryRevision": revision, "packages": len(root["packages"]), "versions": count}


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("--base-url", required=True)
    parser.add_argument("--expected-revision", required=True)
    args = parser.parse_args()
    for attempt in range(12):
        try:
            print(json.dumps(check(args.base_url, args.expected_revision)))
            return
        except Exception as error:
            if attempt == 11:
                raise
            print(json.dumps({"attempt": attempt + 1, "error": str(error)}), flush=True)
            time.sleep(5)


if __name__ == "__main__":
    main()
