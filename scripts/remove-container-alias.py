"""Remove one GHCR version alias without deleting a shared tagged manifest.

Run manually via remove-container-alias.yml with the publishing repository's
package admin token. No layers or retained manifests are deleted. GHCR's REST
delete endpoint operates on a whole package version, so first detach the alias
onto a unique copy of the manifest and delete only that isolated version.
"""
import base64
import hashlib
import json
import os
import re
import time
import urllib.error
import urllib.request
import uuid

PACKAGE = "iannil/huan"
API = "https://api.github.com/users/iannil/packages/container/huan/versions"
REGISTRY = "https://ghcr.io/v2/" + PACKAGE + "/manifests/"
ACCEPT = "application/vnd.oci.image.index.v1+json, application/vnd.docker.distribution.manifest.list.v2+json"


def request(url, headers, method="GET", data=None):
    req = urllib.request.Request(url, headers=headers, method=method, data=data)
    with urllib.request.urlopen(req, timeout=30) as response:
        return response.read(), response.headers


def main():
    tag = os.environ["REMOVE_VERSION"]
    if not re.fullmatch(r"(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)", tag):
        raise ValueError("Expected an unprefixed numeric semver")
    api_headers = {"Authorization": "Bearer " + os.environ["GH_TOKEN"],
                   "Accept": "application/vnd.github+json"}

    def versions():
        result = []
        for page in range(1, 101):
            batch = json.loads(request(API + f"?per_page=100&page={page}", api_headers)[0])
            result.extend(batch)
            if len(batch) < 100:
                return result
        raise RuntimeError("Version listing exceeded pagination limit")

    # Check package API access before changing any registry tag.
    versions()
    basic = base64.b64encode((os.environ["GH_ACTOR"] + ":" + os.environ["GH_TOKEN"]).encode()).decode()
    token = json.loads(request("https://ghcr.io/token?service=ghcr.io&scope=repository:"
                              + PACKAGE + ":pull,push", {"Authorization": "Basic " + basic})[0])["token"]
    headers = {"Authorization": "Bearer " + token, "Accept": ACCEPT}

    def manifest(name):
        body, meta = request(REGISTRY + name, headers)
        digest = "sha256:" + hashlib.sha256(body).hexdigest()
        assert digest == meta["Docker-Content-Digest"]
        return body, digest

    retained = {name: manifest(name)[1] for name in ("v" + tag, "latest")}
    try:
        original, digest = manifest(tag)
    except urllib.error.HTTPError as error:
        if error.code == 404:
            print(f"{tag} already absent; retained tags exist: {retained}")
            return
        raise
    if digest != retained["v" + tag]:
        raise RuntimeError("Alias differs from its v-prefixed release; refusing to modify")
    document = json.loads(original)
    if document.get("mediaType") != "application/vnd.oci.image.index.v1+json":
        raise RuntimeError("Expected an OCI index; refusing to modify")
    document.setdefault("annotations", {})["io.huan.alias-cleanup"] = str(uuid.uuid4())
    isolated = json.dumps(document, separators=(",", ":")).encode()
    isolated_digest = "sha256:" + hashlib.sha256(isolated).hexdigest()
    request(REGISTRY + tag, {**headers, "Content-Type": document["mediaType"]}, "PUT", isolated)
    assert manifest(tag)[1] == isolated_digest
    print(f"Detached {tag} from {digest} to {isolated_digest}")

    victim = None
    for _ in range(12):
        matches = [v for v in versions() if v["name"] == isolated_digest]
        if matches:
            assert len(matches) == 1
            victim = matches[0]
            break
        time.sleep(5)
    if victim is None:
        raise RuntimeError("Isolated version not visible yet; no version was deleted")
    assert victim["metadata"]["container"]["tags"] == [tag], "Refusing to delete a shared version"
    for name, expected in retained.items():
        assert manifest(name)[1] == expected, "Retained tag changed; aborting deletion"
    # Re-read the exact version just before deletion to catch unexpected tagging.
    current = json.loads(request(API + "/" + str(victim["id"]), api_headers)[0])
    assert current["name"] == isolated_digest
    assert current["metadata"]["container"]["tags"] == [tag]
    request(API + "/" + str(victim["id"]), api_headers, "DELETE")
    for name, expected in retained.items():
        assert manifest(name)[1] == expected
    try:
        manifest(tag)
    except urllib.error.HTTPError as error:
        if error.code != 404:
            raise
    else:
        raise RuntimeError("Deleted alias still resolves")
    print(f"Removed {tag}; retained manifests unchanged: {retained}")


if __name__ == "__main__":
    main()
