# Changelog

Release notes are generated and maintained via **release-please** based on **Conventional Commits**.

## Unreleased

## 0.1.0 (2026-01-29)


### ⚠ BREAKING CHANGES

* **core:** Improve OIDC/JWT bootstrap, update strategy configuration and configuration ergonomics ([#73](https://github.com/dc-tec/openbao-operator/issues/73))
* **core:** remove Sentinel drift detection (VAP hardening) ([#39](https://github.com/dc-tec/openbao-operator/issues/39))
* **upgrade:** simplify blue/green cutover and split rolling strategy ([#37](https://github.com/dc-tec/openbao-operator/issues/37))
* **config:** openbaocluster config renderer
* **upgrade:** upgrade manager; blue/green upgrades
* **controller:** openbaocluster refactor; sentinel improvements

### Features

* **api:** improve sentinel observability ([b9d4168](https://github.com/dc-tec/openbao-operator/commit/b9d41686964165291a974d900d9050d8be8983c0))
* **backup;restore:** azure blob storage and GCS support as backup provider ([#71](https://github.com/dc-tec/openbao-operator/issues/71)) ([e8a2f2d](https://github.com/dc-tec/openbao-operator/commit/e8a2f2dd68b4af96136d0e387e9199e934a74c82))
* **bluegreen:** blue/green traffic switching improvements ([5e5f815](https://github.com/dc-tec/openbao-operator/commit/5e5f8157e52dd7dfcacd07565cd35270c0ec3f20))
* **charts:** operator helm chart ([c00ff58](https://github.com/dc-tec/openbao-operator/commit/c00ff58ab1d39b64919acad5456ae221c8b69fc1))
* **config:** structured config for self-init ([abf2259](https://github.com/dc-tec/openbao-operator/commit/abf22590241d1a559bdba857440f0918760a78a4))
* **controller;chart;rbac:** controller hardening, Helm sync automation, and RBAC race fix ([#40](https://github.com/dc-tec/openbao-operator/issues/40)) ([c9dd0b5](https://github.com/dc-tec/openbao-operator/commit/c9dd0b54857a60d2dfe47bcc10d4a75929412a27))
* **controller:** add extra metrics ([3ed3915](https://github.com/dc-tec/openbao-operator/commit/3ed3915ad5d37349891bbc0abadccca7ce0b0643))
* **controller:** improve event filtering using centralized predicates ([968df6c](https://github.com/dc-tec/openbao-operator/commit/968df6c7c58cd7fb95793208605c4ae2f8fe4e8d))
* **controller:** single tenancy support ([49b7327](https://github.com/dc-tec/openbao-operator/commit/49b7327caed9394e89999023a4cd1f2488faf2a4))
* **core:** blue/green upgrades ([1a6783e](https://github.com/dc-tec/openbao-operator/commit/1a6783eeb1cb45933d5cc146c81644fca26ccc11))
* **core:** cluster lifecycle hardening; e2e suite refactor ([#72](https://github.com/dc-tec/openbao-operator/issues/72)) ([3de5142](https://github.com/dc-tec/openbao-operator/commit/3de5142367e0076a169f1ebb14497c150dbf5722))
* **core:** enable Raft Autopilot for automatic dead server cleanup ([#44](https://github.com/dc-tec/openbao-operator/issues/44)) ([61aa711](https://github.com/dc-tec/openbao-operator/commit/61aa7115390c8cd9143f9fd4f985414c2756b909))
* **core:** helm manifest values and templates ([6060fbd](https://github.com/dc-tec/openbao-operator/commit/6060fbd04cfb36caadd97718f604fee4250f43e3))
* **core:** Improve OIDC/JWT bootstrap, update strategy configuration and configuration ergonomics ([#73](https://github.com/dc-tec/openbao-operator/issues/73)) ([446e494](https://github.com/dc-tec/openbao-operator/commit/446e4949febbb3155aa999b2d53a720f971e8db5))
* **core:** introduce restore CRD ([4d19b72](https://github.com/dc-tec/openbao-operator/commit/4d19b72b5c74b337b61776f58f0d8f6ff711e8a9))
* **core:** introduce structured error types ([0b17ae1](https://github.com/dc-tec/openbao-operator/commit/0b17ae13e63ac49ac33d066df146ddb3190c6c40))
* **core:** make JWT audience configurable and plumb JWT bootstrap config across backup/upgrade/restore ([#57](https://github.com/dc-tec/openbao-operator/issues/57)) ([3057c61](https://github.com/dc-tec/openbao-operator/commit/3057c61293920b718d3dd5ece951858b77f5b1c6))
* **core:** OpenShift compatibility support ([#62](https://github.com/dc-tec/openbao-operator/issues/62)) ([47d7770](https://github.com/dc-tec/openbao-operator/commit/47d7770854a52d3113294ecc9cd667d8b54acd77))
* **e2e:** end-to-end testing ([47bed1f](https://github.com/dc-tec/openbao-operator/commit/47bed1fa19b3beadf1cbb339a16456aa0b519359))
* **infra;controller:** implement support for online PVC expansion of running OpenBao Clusters ([#75](https://github.com/dc-tec/openbao-operator/issues/75)) ([42fabd3](https://github.com/dc-tec/openbao-operator/commit/42fabd30c6ef0d5ec4ababe85f74fc8d37cc1810))
* **infra:** Expose listenerName field for Gateway API HTTPRoute targeting ([#30](https://github.com/dc-tec/openbao-operator/issues/30)) ([5babd3f](https://github.com/dc-tec/openbao-operator/commit/5babd3f8a2b44c8135b8c1e2ea75a31062bc42e9))
* **infra:** improve hardened and ACME deployments ([#63](https://github.com/dc-tec/openbao-operator/issues/63)) ([d40600e](https://github.com/dc-tec/openbao-operator/commit/d40600effacb689a89c0f52aee1f74e74129117e))
* **infra:** make DNS namespace configurable in NetworkPolicies ([#58](https://github.com/dc-tec/openbao-operator/issues/58)) ([a675dfa](https://github.com/dc-tec/openbao-operator/commit/a675dfad6c52c030e7c265ebf60836b976957d26))
* **infra:** operator security hardening ([34e703f](https://github.com/dc-tec/openbao-operator/commit/34e703fc57d8915a68c8683f2cd34006a0316505))
* **infra:** standardize sub reconciler pattern ([ae79ef5](https://github.com/dc-tec/openbao-operator/commit/ae79ef5d82cf3c2510b9e23150639d35d73a810a))
* **manifests:** admission validation policies; backup auth ([a76541d](https://github.com/dc-tec/openbao-operator/commit/a76541d5d6770838f69d8b203ff2b4785b23c7a5))
* **manifests:** install manifest ([ffc63c6](https://github.com/dc-tec/openbao-operator/commit/ffc63c669bd13c930c8e8f11ce465298e4ab4c0d))
* **manifests:** optional sentinel deployment for quicker reconcile ([081a17a](https://github.com/dc-tec/openbao-operator/commit/081a17a02060db9fb188477620fbd76d1c55522e))
* **manifests:** security; rbac; backup and upgrade improvements ([89a5ee9](https://github.com/dc-tec/openbao-operator/commit/89a5ee9e1f2f95ab4c374f7831e48167a4b1303b))
* **manifests:** self-service tenant onboarding ([2a8d4d0](https://github.com/dc-tec/openbao-operator/commit/2a8d4d03bfdd53b86af93dbf4b6b4be9c9fcc9a7))
* **manifests:** structured configuration ([503961d](https://github.com/dc-tec/openbao-operator/commit/503961d790f996e1de193b321d005d2d8dcc0d4d))
* **manifests:** wire-in image verification for all components ([d94d1f9](https://github.com/dc-tec/openbao-operator/commit/d94d1f9d14c81bd994124fc964e69787045fb646))
* **policy:** enforce Hardened profile requires replicas &gt;= 3 via VAP ([#23](https://github.com/dc-tec/openbao-operator/issues/23)) ([c15ab9f](https://github.com/dc-tec/openbao-operator/commit/c15ab9fd1421b613e138861a962f51cd76b721b3))
* **provisioner:** configurable tenant resource quotas ([#50](https://github.com/dc-tec/openbao-operator/issues/50)) ([4c6fc29](https://github.com/dc-tec/openbao-operator/commit/4c6fc2915cb821547129a6c9b8e1ed73e42fd500))
* **restore:** add RBAC for restore jobs and validate authentication ([#16](https://github.com/dc-tec/openbao-operator/issues/16)) ([e7772a1](https://github.com/dc-tec/openbao-operator/commit/e7772a146482c9626c545bddff185b9a2f687c1b))
* **security:** Add admission-time protections for SSRF, TLS secrets, and tenant self-service ([#51](https://github.com/dc-tec/openbao-operator/issues/51)) ([ae2f86c](https://github.com/dc-tec/openbao-operator/commit/ae2f86c851b1369676cee536b37dd934c8ef0d0a))
* **security:** add operatorimageVerification field to CRD to allow separate verification of both OpenBao and Operator images ([#8](https://github.com/dc-tec/openbao-operator/issues/8)) ([4c1b8cc](https://github.com/dc-tec/openbao-operator/commit/4c1b8cccd1d2c47618c29efa3d08c54535da421c))
* **security:** cosign keyless image verification ([0c60a60](https://github.com/dc-tec/openbao-operator/commit/0c60a60a530904695241707aba96cae7edf8390f))
* **test:** tlsroute; monitoring; backup/upgrade ([bc8497a](https://github.com/dc-tec/openbao-operator/commit/bc8497a112177ecf837ad3c935af4c90945caafb))
* **upgrade:** harden backup and restore flows ([cb542ab](https://github.com/dc-tec/openbao-operator/commit/cb542ab466e29ddbbf61460ebd9368891aa9e359))
* **upgrade:** improve upgrade manager stability by using SSA for status updates and make pre-upgrade backup job names deterministic ([#17](https://github.com/dc-tec/openbao-operator/issues/17)) ([78f6124](https://github.com/dc-tec/openbao-operator/commit/78f6124b7e3545149b86a167165fb081b7c810ac))
* **vap:** harden OpenBaoRestore VAP guardrails + allow default backup executor image ([#76](https://github.com/dc-tec/openbao-operator/issues/76)) ([93524c8](https://github.com/dc-tec/openbao-operator/commit/93524c8b91563bd5bee91caf2ef0d9360d0a2b04))


### Bug Fixes

* **admission:** add admission check ([50d3af0](https://github.com/dc-tec/openbao-operator/commit/50d3af0aa06773e5ea5ee98a1194cba7c9f98b1e))
* **admission:** implement security/rbac improvements ([95cd1b2](https://github.com/dc-tec/openbao-operator/commit/95cd1b246c2eacb18e9fa8da977a44ee7faf1313))
* **api:** switch SecretReference to LocalObjectReference ([c3b8fef](https://github.com/dc-tec/openbao-operator/commit/c3b8fefd41e8f06b1b4456f66861974d06de4428))
* **auth:** harden OIDC discovery and add least-privilege RBAC + admission guardrails ([#86](https://github.com/dc-tec/openbao-operator/issues/86)) ([d128a5d](https://github.com/dc-tec/openbao-operator/commit/d128a5d653aa504bbaaadaf48dbd240fc8c7c8da))
* **backup:** backup ([8bdc5fa](https://github.com/dc-tec/openbao-operator/commit/8bdc5fa0f00affcc6ca8c172f64bdb557a994b54))
* **backup:** make sure backup jobs are idempotent ([#47](https://github.com/dc-tec/openbao-operator/issues/47)) ([8e2ec6f](https://github.com/dc-tec/openbao-operator/commit/8e2ec6f058928a169718908b3e7fa38150ffcf80))
* **backup:** manual / scheduled backups ([f68172e](https://github.com/dc-tec/openbao-operator/commit/f68172e4800ce383d8a5b40e910465f6ad1ce86c))
* **backup:** pod security context hardening for init and backup containers ([cec43e6](https://github.com/dc-tec/openbao-operator/commit/cec43e6c7fa1e080ad4ec4d223bcb61d2106bbf2))
* **backup:** remove unused function ([556161f](https://github.com/dc-tec/openbao-operator/commit/556161f542a71570fb94660a4d986a51df660a84))
* **backup:** upgrade paths ([e2bb9b5](https://github.com/dc-tec/openbao-operator/commit/e2bb9b5ceded236632ce89eee43a001efc0dca70))
* **chart:** sync helm chart ([9c22829](https://github.com/dc-tec/openbao-operator/commit/9c228297ace116396f351290620eb44991739d57))
* **chart:** sync helm chart ([#7](https://github.com/dc-tec/openbao-operator/issues/7)) ([507c364](https://github.com/dc-tec/openbao-operator/commit/507c36400b8f83b75e614df3fd34fcddd0e12283))
* **controller:** persist initialized status ([c2ebbd1](https://github.com/dc-tec/openbao-operator/commit/c2ebbd1b6701982fbf5881d71c5a073d35f9854d))
* **controller:** Prevent data loss by orphaning secrets when DeletionPolicy is Retain ([#11](https://github.com/dc-tec/openbao-operator/issues/11)) ([0899cfa](https://github.com/dc-tec/openbao-operator/commit/0899cfa44d53deea6aaf65343d44b61c6a488168))
* **controller:** prevent OpenBaoCluster resourceVersion churn ([#49](https://github.com/dc-tec/openbao-operator/issues/49)) ([c0e4fe8](https://github.com/dc-tec/openbao-operator/commit/c0e4fe88c628cec4cab6ed6cd1bc053378f27d1e))
* **controller:** remove force ownership of status ([#70](https://github.com/dc-tec/openbao-operator/issues/70)) ([e59e5da](https://github.com/dc-tec/openbao-operator/commit/e59e5da6d22ea82dde7c8c272447e4744991b51e))
* **controller:** strengthen status updates with patching ([6c54a5e](https://github.com/dc-tec/openbao-operator/commit/6c54a5e4505c8ff17756d8cc477b75641eaebedc))
* **controller:** timeout for image verification ([cbcd9cf](https://github.com/dc-tec/openbao-operator/commit/cbcd9cf753ee6b33d0167d8195cfff69a13e966c))
* **core:** add temporary transient error ([e0aeb21](https://github.com/dc-tec/openbao-operator/commit/e0aeb2146e9e713226c8aeecab06508d69295b2d))
* **core:** centralize constants into internal/constants ([058b0a3](https://github.com/dc-tec/openbao-operator/commit/058b0a3b5abfc1733d49b6dbcbf66e5a4fbb3be4))
* **core:** check token existence ([f4669f5](https://github.com/dc-tec/openbao-operator/commit/f4669f5b2cc2fe844e02e1282e8e3e8e12d5763a))
* **core:** decouple openbao client logic ([d3a0acc](https://github.com/dc-tec/openbao-operator/commit/d3a0acc6323aa7cda1e20b47189576f3a094bb0b))
* **core:** improve container status checking ([e357dcc](https://github.com/dc-tec/openbao-operator/commit/e357dcc0fd385adb4b6a400eaca3cd84ef52bcc4))
* **core:** rbac and admission hardening ([477be64](https://github.com/dc-tec/openbao-operator/commit/477be6472cd6d45324b2ec879a70d50bd10fcf2f))
* **deps:** resolve security vulnerabilities in go-tuf/v2 and rekor dependencies ([#74](https://github.com/dc-tec/openbao-operator/issues/74)) ([ecbfba8](https://github.com/dc-tec/openbao-operator/commit/ecbfba80715689bf0eb1689ec370befbfad6cd83))
* **e2e:** sentinel drift detection robustness ([648f3df](https://github.com/dc-tec/openbao-operator/commit/648f3df3b08e71633f172993a22ecaf0559acfdb))
* **e2e:** unused param ([b7a9c02](https://github.com/dc-tec/openbao-operator/commit/b7a9c0294172e5d52c048e117a874239ffb6d10a))
* **images:** fail-fast on missing OPERATOR_VERSION environment variable ([#25](https://github.com/dc-tec/openbao-operator/issues/25)) ([1a42097](https://github.com/dc-tec/openbao-operator/commit/1a42097c8fd80bfe773682865c1119b29ca77d02))
* Implement versioned default images for backup, upgrade, and init container ([#14](https://github.com/dc-tec/openbao-operator/issues/14)) ([1b34f78](https://github.com/dc-tec/openbao-operator/commit/1b34f785009750a2667293d31334260fee04716d))
* **infra:** add IPv6/dual-stack support for listener binding and development egress rules ([#56](https://github.com/dc-tec/openbao-operator/issues/56)) ([7bfdb41](https://github.com/dc-tec/openbao-operator/commit/7bfdb41840bed338cbfcede82be3aea6642a7a53))
* **infra:** exclude job pods from pdb ([#9](https://github.com/dc-tec/openbao-operator/issues/9)) ([825a191](https://github.com/dc-tec/openbao-operator/commit/825a1916d68a6a0bb09c4f46c1251cf2af9cd159))
* **infra:** improve initialization robustness by treating transient Secret/RBAC errors as retriable and hardening root-token creation ([#55](https://github.com/dc-tec/openbao-operator/issues/55)) ([f760ac5](https://github.com/dc-tec/openbao-operator/commit/f760ac5c17bd99f747e8c3dc637bdcee1b4cb511))
* **infra:** resolve BackendTLSPolicy mismatch and cleanup stale services after Blue/Green upgrade ([#10](https://github.com/dc-tec/openbao-operator/issues/10)) ([7052a54](https://github.com/dc-tec/openbao-operator/commit/7052a54145a4d9ac1a1d9ed3b7fdb1cc8de994a2))
* **infra:** stop apiserver endpoint autodetection; use service VIP allow-list with optional endpoint IPs ([#54](https://github.com/dc-tec/openbao-operator/issues/54)) ([d73179a](https://github.com/dc-tec/openbao-operator/commit/d73179a434428bb787684791d1de88dc778f138f))
* **init:** retrty writing root token to secret to handle transient cr… ([#84](https://github.com/dc-tec/openbao-operator/issues/84)) ([e100176](https://github.com/dc-tec/openbao-operator/commit/e1001769b05fbccae2c861b586dd3eac3eaefd8c))
* **kube:** add job check ([a7439a9](https://github.com/dc-tec/openbao-operator/commit/a7439a9fe060a4710deda76bea6b7bfafde18020))
* **manifests:** improve operator rbac ([8a17db3](https://github.com/dc-tec/openbao-operator/commit/8a17db3dc103b294e7b602c0a69ff1088e5393e7))
* **manifests:** make JWT auth bootstrap a opt-in feature ([ded02a3](https://github.com/dc-tec/openbao-operator/commit/ded02a3173c672e7cbc03f5e993635e1cb345663))
* **manifests:** operator namespace detection ([139450a](https://github.com/dc-tec/openbao-operator/commit/139450a401beb7b292aad7962e84e6dcfb109098))
* **manifests:** rbac; upgrade deps ([8b7d4e8](https://github.com/dc-tec/openbao-operator/commit/8b7d4e85c080473fcc01d53e103ffc3891d0949a))
* **manifests:** secure defaults and profiles ([6617383](https://github.com/dc-tec/openbao-operator/commit/66173839968834008119e07cf29cc99188ef8121))
* **security:** implement image verification LRU cache; docker auth handeling ([#18](https://github.com/dc-tec/openbao-operator/issues/18)) ([a4b7203](https://github.com/dc-tec/openbao-operator/commit/a4b720313ec7fa40a7b0123de4bbbbe090441c0e))
* **security:** performance issue image verification by reording cache lookups ([#12](https://github.com/dc-tec/openbao-operator/issues/12)) ([a5ca5eb](https://github.com/dc-tec/openbao-operator/commit/a5ca5eb1268d9afe98d8bcc0ce6c3dda0efde20c))
* **sentinel:** prevent noisy neighbors and thundering herd behavior ([57eb7bd](https://github.com/dc-tec/openbao-operator/commit/57eb7bdfd9b714e2d64c0954d5a36c260dde7efa))
* **sentinel:** rely on uuids instead of timestamps as sentinel triggerid ([#6](https://github.com/dc-tec/openbao-operator/issues/6)) ([f88b697](https://github.com/dc-tec/openbao-operator/commit/f88b697f6dc13f19cf9a00d2764a4ed0be58868d))
* **upgrade:** add metrics for upgrade ([936d71e](https://github.com/dc-tec/openbao-operator/commit/936d71edca1f111a40c8a04bd32910459c24fc93))
* **upgrade:** improve upgrade manager stability ([#13](https://github.com/dc-tec/openbao-operator/issues/13)) ([c6a1b34](https://github.com/dc-tec/openbao-operator/commit/c6a1b34a515e7ed4201d61cd2b564ba2b0a9b5bf))
* **upgrade:** revert partition update to MergeFrom to fix StatefulSet validation ([#52](https://github.com/dc-tec/openbao-operator/issues/52)) ([504c319](https://github.com/dc-tec/openbao-operator/commit/504c31970030519ed602f16ebc3d7be5b339d32c))
* **upgrade:** use SSA for upgrade manager ([d0c289c](https://github.com/dc-tec/openbao-operator/commit/d0c289ce76686f7329e79cbbbfdc29b172446c74))
* **vap:** require self init requests when self initialization is enabled ([#82](https://github.com/dc-tec/openbao-operator/issues/82)) ([c572aaa](https://github.com/dc-tec/openbao-operator/commit/c572aaa392ecc8c8f6dccdee5203a964055a6106))
* **vap:** stuck Job deletions by allowing GC Job-finalizer updates in lock-managed-resource-mutations VAP ([#53](https://github.com/dc-tec/openbao-operator/issues/53)) ([0c56a87](https://github.com/dc-tec/openbao-operator/commit/0c56a8726c3a972566fc4a93b8a8d3d9bbd99ae7))


### Code Refactoring

* **config:** openbaocluster config renderer ([a230262](https://github.com/dc-tec/openbao-operator/commit/a230262c4795566c21ad58a65b74364e7cdd36b6))
* **controller:** openbaocluster refactor; sentinel improvements ([9d0de98](https://github.com/dc-tec/openbao-operator/commit/9d0de984d9681d53f4c5569ff84443ae46e2bad5))
* **core:** remove Sentinel drift detection (VAP hardening) ([#39](https://github.com/dc-tec/openbao-operator/issues/39)) ([d289cf2](https://github.com/dc-tec/openbao-operator/commit/d289cf262213ab13ca3c9e3631df1d4845ee6fc7))
* **upgrade:** simplify blue/green cutover and split rolling strategy ([#37](https://github.com/dc-tec/openbao-operator/issues/37)) ([7453e23](https://github.com/dc-tec/openbao-operator/commit/7453e23880b1edbfa0c825d6982c29893d4ac08d))
* **upgrade:** upgrade manager; blue/green upgrades ([2ba56a4](https://github.com/dc-tec/openbao-operator/commit/2ba56a426caa12a79a069700b0b2a4ede44156e1))

## 0.1.0

Initial release.

### Highlights

- Core OpenBao operator (controller + provisioner).
- Helm chart and install manifests (including CRDs).
- Backup/restore and upgrade workflows (including rolling and blue/green).
- Admission and supply-chain guardrails for hardened environments.
- E2E suite and CI pipelines for multi-Kubernetes validation.
