---
title: Contribute
description: Local setup, standards, testing, CI, supply-chain controls, and release guidance for OpenBao Operator contributors.
eyebrow: Contributor handbook
weight: 7
hideChildren: true
verifiedBy:
  - mk/development.mk
  - mk/ci.mk
aliases:
  - /latest/contributing/
  - /dev/contributing/
---

Contributions may use AI assistance, but the submitter remains responsible for correctness, security, testing, and understanding the change.

## Start a change

<ol class="steps">
  <li>Read the repository rules and the guide for the code or documentation surface you will change.</li>
  <li>Bootstrap repo-managed tools and verify the workstation before editing.</li>
  <li>Choose the smallest test depth that proves the behavior, then expand with the risk.</li>
  <li>Keep generated artifacts synchronized and review every generated diff.</li>
  <li>Use a scoped conventional commit and hand CI evidence to the reviewer.</li>
</ol>

{{< command label="verify" title="PR-equivalent local gate" >}}
devenv test
devenv tasks run operator:bootstrap
devenv tasks run operator:doctor
devenv tasks run operator:ci-core
{{< /command >}}

## Contributor routes

<div class="link-grid">
  <a href="{{< relref \"/contribute/setup.md\" >}}"><strong>Set up a workstation</strong><p>Bootstrap tools and choose a local development loop.</p></a>
  <a href="{{< relref \"/contribute/standards.md\" >}}"><strong>Follow project standards</strong><p>Apply the Go, controller, security, documentation, and generated-file rules.</p></a>
  <a href="{{< relref \"/contribute/testing.md\" >}}"><strong>Test a change</strong><p>Choose unit, integration, E2E, fuzz, performance, or platform validation.</p></a>
  <a href="{{< relref \"/contribute/ci.md\" >}}"><strong>Run CI-equivalent checks</strong><p>Map local evidence to pull-request and scheduled workflows.</p></a>
  <a href="{{< relref \"/contribute/supply-chain.md\" >}}"><strong>Protect the supply chain</strong><p>Review dependency, provenance, distribution, and governance controls.</p></a>
  <a href="{{< relref \"/contribute/release.md\" >}}"><strong>Prepare a release</strong><p>Build once, promote by digest, publish, and retain evidence.</p></a>
  <a href="{{< relref \"/contribute/incident-response.md\" >}}"><strong>Respond to a publishing incident</strong><p>Contain, investigate, rotate trust, and recover release automation.</p></a>
</div>
