---
title: API reference
description: Generated fields, defaults, and validation for the OpenBao Operator custom resources.
eyebrow: Reference
weight: 1
verifiedBy:
  - api/v1alpha1 at 590ae5a92364ae88c294dda46868c3409ddfce67
  - docs/reference/api.md at 590ae5a92364ae88c294dda46868c3409ddfce67
---

Use these pages when you need the exact field name, type, default, or validation rule for an operator custom resource.

The repository generates the source reference from `api/v1alpha1`. The synchronization step pins the Next source ref
and writes one page per top-level resource so each page has a useful table of contents and unique anchors.

{{< callout type="warning" title="Apply the matching CRDs" >}}
The API reference is generated from the exact `main` commit recorded above. Apply the CRDs from that source checkout,
or use the edge channel only when `metadata.json` records the same `sha`. The edge publisher can briefly lag a newly
merged commit. Do not combine this schema with a stable operator release.
{{< /callout >}}
