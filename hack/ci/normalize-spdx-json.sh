#!/usr/bin/env bash

set -euo pipefail

: "${SOURCE_DATE_EPOCH:?SOURCE_DATE_EPOCH is required}"

if [[ "$#" -lt 1 ]]; then
  echo "usage: $0 <sbom.spdx.json> [more sbom files...]" >&2
  exit 1
fi

python3 - "${SOURCE_DATE_EPOCH}" "$@" <<'PY'
import copy
import datetime
import hashlib
import json
import sys
from pathlib import Path


def sort_list(entries, keys):
    if not isinstance(entries, list):
        return entries

    def key_fn(item):
        if not isinstance(item, dict):
            return (str(item),)
        return tuple(str(item.get(k, "")) for k in keys)

    return sorted(entries, key=key_fn)


def canonicalize(node):
    if isinstance(node, dict):
        out = {k: canonicalize(v) for k, v in node.items()}
        out["packages"] = sort_list(out.get("packages"), ("SPDXID", "name", "versionInfo"))
        out["relationships"] = sort_list(
            out.get("relationships"),
            ("spdxElementId", "relationshipType", "relatedSpdxElement", "comment"),
        )
        out["files"] = sort_list(out.get("files"), ("SPDXID", "fileName"))
        out["annotations"] = sort_list(out.get("annotations"), ("SPDXID", "annotationDate", "comment"))
        out["hasExtractedLicensingInfos"] = sort_list(
            out.get("hasExtractedLicensingInfos"),
            ("licenseId", "name", "comment"),
        )
        return out
    if isinstance(node, list):
        return [canonicalize(v) for v in node]
    return node


def canonical_json(obj):
    return json.dumps(obj, sort_keys=True, separators=(",", ":"), ensure_ascii=False)


def normalize_document(doc, created):
    doc = canonicalize(doc)
    creation_info = doc.setdefault("creationInfo", {})
    if isinstance(creation_info, dict):
        creation_info["created"] = created

    namespace_seed = copy.deepcopy(doc)
    namespace_seed.pop("documentNamespace", None)
    seed_creation = namespace_seed.get("creationInfo")
    if isinstance(seed_creation, dict):
        seed_creation.pop("created", None)
    namespace_seed = canonicalize(namespace_seed)

    seed_hash = hashlib.sha256(canonical_json(namespace_seed).encode("utf-8")).hexdigest()
    doc["documentNamespace"] = f"https://openbao-operator.dev/sbom/{seed_hash}"

    return canonicalize(doc)


source_date_epoch = int(sys.argv[1])
created = datetime.datetime.fromtimestamp(
    source_date_epoch, datetime.timezone.utc
).strftime("%Y-%m-%dT%H:%M:%SZ")

for sbom_path in sys.argv[2:]:
    path = Path(sbom_path)
    data = json.loads(path.read_text(encoding="utf-8"))
    normalized = normalize_document(data, created)
    path.write_text(canonical_json(normalized) + "\n", encoding="utf-8")
PY
