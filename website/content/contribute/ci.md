---
title: Run CI-equivalent checks
description: Map local validation to pull-request, main, nightly, and release workflow lanes.
eyebrow: Contribute
weight: 4
verifiedBy:
  - devenv.nix
  - devenv.yaml
  - devenv.lock
  - hack/dev/verify-devenv.sh
  - .github/workflows/ci.yml
  - .github/workflows/nightly.yml
  - .github/workflows/release.yml
  - test/e2e/suites.yaml
---

Pull-request CI routes checks by changed path and risk. Nightly and release workflows broaden compatibility, lifecycle, provenance, and reproducibility coverage.

{{< command label="verify" title="Run the local pull-request baseline" >}}
devenv test
devenv tasks run operator:bootstrap
devenv tasks run operator:doctor
devenv tasks run operator:ci-core
{{< /command >}}

The advisory `Devenv Contract` job runs on toolchain changes and pushes to `main`. It proves that the pinned base and
editor-profile shells can be constructed on the CI runner and that their version contracts pass. Base and editor
duration and closure measurements remain separate. The job pulls from the public project and Devenv caches. It
publishes newly built paths to the project cache only from pushes to `main` when the repository has a per-cache
`CACHIX_AUTH_TOKEN`; pull requests are always read-only. The existing required jobs remain the merge authority while
the project migrates their setup steps incrementally.

| CI concern | Local entry point |
| --- | --- |
| Pinned environment contract | `devenv test` |
| Core quality gates | `devenv tasks run operator:ci-core` |
| API inventory and released CRD compatibility | `make verify-api-contract` |
| Hugo documentation and redirects | `make docs-build` |
| Vendored dependencies and licenses | `make verify-vendor`, `make license-check` |
| Static and filesystem security | `make semgrep-ci`, `make security-ci` |
| Focused cluster behavior | `make test-e2e-ci`, `make helm-e2e-smoke` |
| Prior-stable operator upgrade | `make test-e2e-operator-upgrade` |
| Existing platform cluster | `make test-e2e-existing` |

Docs-only changes normally avoid broad cluster E2E. Backup, upgrade, security, provisioner, admission, controller-critical, or seal-path changes expand into targeted shards. Maintainers can request the full E2E set with the repository's CI label.

The required `API Contract` job rejects unclassified API fields, stale inventory snapshots, and breaking or
review-required schema changes against the released 0.5.0 CRDs. It runs for contract-related changes, pushes to
`main`, and manual CI runs. Release validation runs the same gate before image builds and publication.
`make report-crd-compatibility` produces a diagnostic report without weakening the required gate.

The E2E manifest owns suite routing, isolation, parallelism, and supported test versions. Do not duplicate those values in workflow logic. Run `make e2e-ci-matrix` or the nightly matrix commands before changing routing.

{{< callout type="note" title="Publishing requires more than PR CI" >}}
Edge, nightly, prerelease, and stable channels add immutable-subject, provenance, reproducibility, signing, and release-evidence gates. Passing `ci-core` does not authorize publication.
{{< /callout >}}

Continue with [supply-chain controls]({{< relref "/contribute/supply-chain.md" >}}) and [release management]({{< relref "/contribute/release.md" >}}).
