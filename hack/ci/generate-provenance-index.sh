#!/usr/bin/env bash

set -euo pipefail

: "${REPO:?REPO is required (owner/repo)}"
: "${OWNER:?OWNER is required}"
: "${VERSION:?VERSION is required}"
: "${MANAGER_IMAGE:?MANAGER_IMAGE is required}"
: "${MANAGER_DIGEST:?MANAGER_DIGEST is required}"
: "${CONFIG_INIT_IMAGE:?CONFIG_INIT_IMAGE is required}"
: "${CONFIG_INIT_DIGEST:?CONFIG_INIT_DIGEST is required}"
: "${BACKUP_EXECUTOR_IMAGE:?BACKUP_EXECUTOR_IMAGE is required}"
: "${BACKUP_EXECUTOR_DIGEST:?BACKUP_EXECUTOR_DIGEST is required}"
: "${UPGRADE_EXECUTOR_IMAGE:?UPGRADE_EXECUTOR_IMAGE is required}"
: "${UPGRADE_EXECUTOR_DIGEST:?UPGRADE_EXECUTOR_DIGEST is required}"
: "${CHART_DIGEST:?CHART_DIGEST is required}"

INDEX_PATH="${INDEX_PATH:-dist/provenance-index.json}"

python3 - <<'PY'
import datetime
import glob
import hashlib
import json
import os
from pathlib import Path

repo = os.environ["REPO"]
owner = os.environ["OWNER"]
version = os.environ["VERSION"]
index_path = Path(os.environ.get("INDEX_PATH", "dist/provenance-index.json"))
index_path.parent.mkdir(parents=True, exist_ok=True)


def sha256_file(path: Path) -> str:
    h = hashlib.sha256()
    with path.open("rb") as f:
        for chunk in iter(lambda: f.read(8192), b""):
            h.update(chunk)
    return h.hexdigest()


def api_attestation_uri(digest: str) -> str:
    value = digest.replace("sha256:", "")
    return f"https://api.github.com/repos/{repo}/attestations/sha256:{value}"


release_files = [
    "dist/install.yaml",
    "dist/crds.yaml",
    "dist/checksums.txt",
    "dist/checksums.txt.bundle",
]
release_files.extend(sorted(glob.glob("dist/sbom-*.spdx.json")))

checksums_subjects = {}
checksums_path = Path("dist/checksums.txt")
if checksums_path.exists():
    for line in checksums_path.read_text(encoding="utf-8").splitlines():
        parts = line.split()
        if len(parts) != 2:
            continue
        digest, file_name = parts
        checksums_subjects[file_name] = digest

artifact_entries = []
for file_name in release_files:
    path = Path(file_name)
    if not path.exists():
        continue
    artifact_entries.append(
        {
            "path": file_name,
            "sha256": sha256_file(path),
            "included_in_checksums_txt": path.name in checksums_subjects,
            "checksums_txt_sha256": checksums_subjects.get(path.name),
        }
    )

images = [
    {
        "name": "openbao-operator",
        "ref": os.environ["MANAGER_IMAGE"],
        "digest": os.environ["MANAGER_DIGEST"],
    },
    {
        "name": "openbao-init",
        "ref": os.environ["CONFIG_INIT_IMAGE"],
        "digest": os.environ["CONFIG_INIT_DIGEST"],
    },
    {
        "name": "openbao-backup",
        "ref": os.environ["BACKUP_EXECUTOR_IMAGE"],
        "digest": os.environ["BACKUP_EXECUTOR_DIGEST"],
    },
    {
        "name": "openbao-upgrade",
        "ref": os.environ["UPGRADE_EXECUTOR_IMAGE"],
        "digest": os.environ["UPGRADE_EXECUTOR_DIGEST"],
    },
]

for image in images:
    image["oci_subject"] = f"{image['ref']}@{image['digest']}"
    image["attestation_api"] = api_attestation_uri(image["digest"])
    image["signing_identity"] = (
        f"https://github.com/{repo}/.github/workflows/release.yml@refs/tags/{version}"
    )
    image["attestation_signer_workflow"] = f"{repo}/.github/workflows/reusable-build.yml"

chart_digest = os.environ["CHART_DIGEST"]
checksums_digest = f"sha256:{sha256_file(checksums_path)}" if checksums_path.exists() else None

index = {
    "schema_version": "v1alpha1",
    "generated_at_utc": datetime.datetime.now(datetime.timezone.utc).isoformat(),
    "release": {
        "repository": repo,
        "owner": owner,
        "tag": version,
        "source_ref": f"refs/tags/{version}",
        "claim": "Targets SLSA Build L3 controls with additional L4-like hardening.",
    },
    "identity_constraints": {
        "oidc_issuer": "https://token.actions.githubusercontent.com",
        "reusable_build_signer_workflow": f"{repo}/.github/workflows/reusable-build.yml",
        "release_signer_workflow": f"{repo}/.github/workflows/release.yml",
    },
    "images": images,
    "chart": {
        "ref": f"ghcr.io/{owner}/charts/openbao-operator",
        "digest": chart_digest,
        "oci_subject": f"ghcr.io/{owner}/charts/openbao-operator@{chart_digest}",
        "attestation_api": api_attestation_uri(chart_digest),
        "signature_identity": (
            f"https://github.com/{repo}/.github/workflows/release.yml@refs/tags/{version}"
        ),
    },
    "release_artifacts": {
        "checksums_txt": {
            "path": "dist/checksums.txt",
            "digest": checksums_digest,
            "attestation_api": api_attestation_uri(checksums_digest) if checksums_digest else None,
            "signature_bundle_path": "dist/checksums.txt.bundle",
        },
        "files": artifact_entries,
    },
}

index_path.write_text(json.dumps(index, indent=2, sort_keys=True) + "\n", encoding="utf-8")
print(f"Wrote {index_path}")
PY
