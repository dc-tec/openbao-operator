# Changelog

Release notes are generated and maintained via **release-please** based on **Conventional Commits**.

## [0.1.0](https://github.com/dc-tec/openbao-operator/compare/0.2.1...0.1.0) (2026-05-18)


### ⚠ BREAKING CHANGES

* **core:** Improve OIDC/JWT bootstrap, update strategy configuration and configuration ergonomics ([#73](https://github.com/dc-tec/openbao-operator/issues/73))
* **core:** remove Sentinel drift detection (VAP hardening) ([#39](https://github.com/dc-tec/openbao-operator/issues/39))
* **upgrade:** simplify blue/green cutover and split rolling strategy ([#37](https://github.com/dc-tec/openbao-operator/issues/37))
* **config:** openbaocluster config renderer
* **upgrade:** upgrade manager; blue/green upgrades
* **controller:** openbaocluster refactor; sentinel improvements

### Features

* **admission:** authorize maintenance through RBAC ([#347](https://github.com/dc-tec/openbao-operator/issues/347)) ([b7c05a7](https://github.com/dc-tec/openbao-operator/commit/b7c05a770bcc97ea1931caf0a3c05919540c38ab))
* **api:** add OpenBaoCluster observedGeneration and printer columns ([#286](https://github.com/dc-tec/openbao-operator/issues/286)) ([1c8f8ae](https://github.com/dc-tec/openbao-operator/commit/1c8f8aeb143fd90ca6452d2f72852c47b14ab5ea))
* **api:** add runtime restart controls ([#348](https://github.com/dc-tec/openbao-operator/issues/348)) ([b1efd34](https://github.com/dc-tec/openbao-operator/commit/b1efd3442c2c5cd0a58c654b749103ab7cf5ac81))
* **api:** improve sentinel observability ([b9d4168](https://github.com/dc-tec/openbao-operator/commit/b9d41686964165291a974d900d9050d8be8983c0))
* **ast-grep:** add policy-driven architecture guardrails with CI enforcement ([#201](https://github.com/dc-tec/openbao-operator/issues/201)) ([1faee9a](https://github.com/dc-tec/openbao-operator/commit/1faee9a6b000d0e68770d7e2894e68d66f13f534))
* **backup;restore:** azure blob storage and GCS support as backup provider ([#71](https://github.com/dc-tec/openbao-operator/issues/71)) ([e8a2f2d](https://github.com/dc-tec/openbao-operator/commit/e8a2f2dd68b4af96136d0e387e9199e934a74c82))
* **bluegreen:** blue/green traffic switching improvements ([5e5f815](https://github.com/dc-tec/openbao-operator/commit/5e5f8157e52dd7dfcacd07565cd35270c0ec3f20))
* **charts:** operator helm chart ([c00ff58](https://github.com/dc-tec/openbao-operator/commit/c00ff58ab1d39b64919acad5456ae221c8b69fc1))
* **config:** structured config for self-init ([abf2259](https://github.com/dc-tec/openbao-operator/commit/abf22590241d1a559bdba857440f0918760a78a4))
* **controller;chart;rbac:** controller hardening, Helm sync automation, and RBAC race fix ([#40](https://github.com/dc-tec/openbao-operator/issues/40)) ([c9dd0b5](https://github.com/dc-tec/openbao-operator/commit/c9dd0b54857a60d2dfe47bcc10d4a75929412a27))
* **controller:** add extra metrics ([3ed3915](https://github.com/dc-tec/openbao-operator/commit/3ed3915ad5d37349891bbc0abadccca7ce0b0643))
* **controller:** single tenancy support ([49b7327](https://github.com/dc-tec/openbao-operator/commit/49b7327caed9394e89999023a4cd1f2488faf2a4))
* **core:** add consistent Kubernetes lifecycle events ([#226](https://github.com/dc-tec/openbao-operator/issues/226)) ([93687af](https://github.com/dc-tec/openbao-operator/commit/93687af087760053b01de76dc6a050e3f5c9e280))
* **core:** add perf baseline harness and gates ([#118](https://github.com/dc-tec/openbao-operator/issues/118)) ([bf91ce2](https://github.com/dc-tec/openbao-operator/commit/bf91ce24ec1de79cb96b1d1a1370938b62195dd7))
* **core:** blue/green upgrades ([1a6783e](https://github.com/dc-tec/openbao-operator/commit/1a6783eeb1cb45933d5cc146c81644fca26ccc11))
* **core:** cluster lifecycle hardening; e2e suite refactor ([#72](https://github.com/dc-tec/openbao-operator/issues/72)) ([3de5142](https://github.com/dc-tec/openbao-operator/commit/3de5142367e0076a169f1ebb14497c150dbf5722))
* **core:** enable Raft Autopilot for automatic dead server cleanup ([#44](https://github.com/dc-tec/openbao-operator/issues/44)) ([61aa711](https://github.com/dc-tec/openbao-operator/commit/61aa7115390c8cd9143f9fd4f985414c2756b909))
* **core:** harden lifecycle contracts and supporting coverage ([#237](https://github.com/dc-tec/openbao-operator/issues/237)) ([44de947](https://github.com/dc-tec/openbao-operator/commit/44de94790ed765a8eb4036490858139b6a8561bd))
* **core:** helm manifest values and templates ([6060fbd](https://github.com/dc-tec/openbao-operator/commit/6060fbd04cfb36caadd97718f604fee4250f43e3))
* **core:** Improve OIDC/JWT bootstrap, update strategy configuration and configuration ergonomics ([#73](https://github.com/dc-tec/openbao-operator/issues/73)) ([446e494](https://github.com/dc-tec/openbao-operator/commit/446e4949febbb3155aa999b2d53a720f971e8db5))
* **core:** introduce restore CRD ([4d19b72](https://github.com/dc-tec/openbao-operator/commit/4d19b72b5c74b337b61776f58f0d8f6ff711e8a9))
* **core:** make JWT audience configurable and plumb JWT bootstrap config across backup/upgrade/restore ([#57](https://github.com/dc-tec/openbao-operator/issues/57)) ([3057c61](https://github.com/dc-tec/openbao-operator/commit/3057c61293920b718d3dd5ece951858b77f5b1c6))
* **core:** OpenShift compatibility support ([#62](https://github.com/dc-tec/openbao-operator/issues/62)) ([47d7770](https://github.com/dc-tec/openbao-operator/commit/47d7770854a52d3113294ecc9cd667d8b54acd77))
* **infra;controller:** implement support for online PVC expansion of running OpenBao Clusters ([#75](https://github.com/dc-tec/openbao-operator/issues/75)) ([42fabd3](https://github.com/dc-tec/openbao-operator/commit/42fabd30c6ef0d5ec4ababe85f74fc8d37cc1810))
* **infra:** add default node and zone spreading for OpenBao StatefulSets ([#214](https://github.com/dc-tec/openbao-operator/issues/214)) ([1d7afc8](https://github.com/dc-tec/openbao-operator/commit/1d7afc8d55ddede8b24207274101a13d4352e98a))
* **infra:** add pod metadata hooks for workload identity ([#216](https://github.com/dc-tec/openbao-operator/issues/216)) ([9bd2546](https://github.com/dc-tec/openbao-operator/commit/9bd2546ccf5caf0024263073635a2a87ad6713c1))
* **infra:** Expose listenerName field for Gateway API HTTPRoute targeting ([#30](https://github.com/dc-tec/openbao-operator/issues/30)) ([5babd3f](https://github.com/dc-tec/openbao-operator/commit/5babd3f8a2b44c8135b8c1e2ea75a31062bc42e9))
* **infra:** improve hardened and ACME deployments ([#63](https://github.com/dc-tec/openbao-operator/issues/63)) ([d40600e](https://github.com/dc-tec/openbao-operator/commit/d40600effacb689a89c0f52aee1f74e74129117e))
* **infra:** make DNS namespace configurable in NetworkPolicies ([#58](https://github.com/dc-tec/openbao-operator/issues/58)) ([a675dfa](https://github.com/dc-tec/openbao-operator/commit/a675dfad6c52c030e7c265ebf60836b976957d26))
* **manifests:** install manifest ([ffc63c6](https://github.com/dc-tec/openbao-operator/commit/ffc63c669bd13c930c8e8f11ce465298e4ab4c0d))
* **manifests:** optional sentinel deployment for quicker reconcile ([081a17a](https://github.com/dc-tec/openbao-operator/commit/081a17a02060db9fb188477620fbd76d1c55522e))
* **manifests:** self-service tenant onboarding ([2a8d4d0](https://github.com/dc-tec/openbao-operator/commit/2a8d4d03bfdd53b86af93dbf4b6b4be9c9fcc9a7))
* **manifests:** structured configuration ([503961d](https://github.com/dc-tec/openbao-operator/commit/503961d790f996e1de193b321d005d2d8dcc0d4d))
* **manifests:** wire-in image verification for all components ([d94d1f9](https://github.com/dc-tec/openbao-operator/commit/d94d1f9d14c81bd994124fc964e69787045fb646))
* **observability:** add metrics, dashboards, e2e assertions; upgrade stability ([#101](https://github.com/dc-tec/openbao-operator/issues/101)) ([d4ce07d](https://github.com/dc-tec/openbao-operator/commit/d4ce07dc4d895381066ca86962fc5758f66dfd33))
* **operator:** add supported single-tenant custom identity install paths ([#239](https://github.com/dc-tec/openbao-operator/issues/239)) ([d41ff74](https://github.com/dc-tec/openbao-operator/commit/d41ff74b33133bd05bb2b2a7dadcaf4e4fe3305a))
* **perf:** refresh kind performance baseline ([#120](https://github.com/dc-tec/openbao-operator/issues/120)) ([69e5366](https://github.com/dc-tec/openbao-operator/commit/69e5366651ac500925336358fb013c0e9650e4f2))
* **policy:** enforce Hardened profile requires replicas &gt;= 3 via VAP ([#23](https://github.com/dc-tec/openbao-operator/issues/23)) ([c15ab9f](https://github.com/dc-tec/openbao-operator/commit/c15ab9fd1421b613e138861a962f51cd76b721b3))
* **provisioner:** configurable tenant resource quotas ([#50](https://github.com/dc-tec/openbao-operator/issues/50)) ([4c6fc29](https://github.com/dc-tec/openbao-operator/commit/4c6fc2915cb821547129a6c9b8e1ed73e42fd500))
* **readreplicas:** add steady-state read replica topology and status ([#361](https://github.com/dc-tec/openbao-operator/issues/361)) ([9a74c14](https://github.com/dc-tec/openbao-operator/commit/9a74c143e9061f42f5c7557af7a7e9b767252926))
* **readreplicas:** integrate read replicas with upgrade and restore workflows ([#362](https://github.com/dc-tec/openbao-operator/issues/362)) ([e8bf8b8](https://github.com/dc-tec/openbao-operator/commit/e8bf8b820c06ccab1fb81a9df25223dfbf4e0666))
* **restore:** add RBAC for restore jobs and validate authentication ([#16](https://github.com/dc-tec/openbao-operator/issues/16)) ([e7772a1](https://github.com/dc-tec/openbao-operator/commit/e7772a146482c9626c545bddff185b9a2f687c1b))
* **security:** Add admission-time protections for SSRF, TLS secrets, and tenant self-service ([#51](https://github.com/dc-tec/openbao-operator/issues/51)) ([ae2f86c](https://github.com/dc-tec/openbao-operator/commit/ae2f86c851b1369676cee536b37dd934c8ef0d0a))
* **security:** add operatorimageVerification field to CRD to allow separate verification of both OpenBao and Operator images ([#8](https://github.com/dc-tec/openbao-operator/issues/8)) ([4c1b8cc](https://github.com/dc-tec/openbao-operator/commit/4c1b8cccd1d2c47618c29efa3d08c54535da421c))
* **security:** expand control-plane audit coverage for startup, operations, and RBAC mutations ([#109](https://github.com/dc-tec/openbao-operator/issues/109)) ([b32dc97](https://github.com/dc-tec/openbao-operator/commit/b32dc97175999aadb84cecf867395a7cca2a6f85))
* **security:** harden image verification and align edge/nightly signed manifest streams ([#112](https://github.com/dc-tec/openbao-operator/issues/112)) ([b755ca3](https://github.com/dc-tec/openbao-operator/commit/b755ca333c4e598cf5904b9e68817ac540393cc5))
* **security:** harden image verification defaults and sign edge/nightly images ([#111](https://github.com/dc-tec/openbao-operator/issues/111)) ([5ffed83](https://github.com/dc-tec/openbao-operator/commit/5ffed83ea179fe14fedba50320425d8e4ce0b30c))
* **security:** harden operator RBAC with ValidatingAdmissionPolicy guardrails ([#100](https://github.com/dc-tec/openbao-operator/issues/100)) ([643fd94](https://github.com/dc-tec/openbao-operator/commit/643fd94af7f0a128bf4f62fa073ffa70ec92af18))
* **security:** tighten operator security and authentication contracts ([#238](https://github.com/dc-tec/openbao-operator/issues/238)) ([7b14fb1](https://github.com/dc-tec/openbao-operator/commit/7b14fb1cc9046cd469451c3d1d8bb4cb0cbb0302))
* **upgrade:** harden backup and restore flows ([cb542ab](https://github.com/dc-tec/openbao-operator/commit/cb542ab466e29ddbbf61460ebd9368891aa9e359))
* **upgrade:** improve upgrade manager stability by using SSA for status updates and make pre-upgrade backup job names deterministic ([#17](https://github.com/dc-tec/openbao-operator/issues/17)) ([78f6124](https://github.com/dc-tec/openbao-operator/commit/78f6124b7e3545149b86a167165fb081b7c810ac))
* **upgrade:** unify manual upgrade requests on OpenBaoCluster ([#228](https://github.com/dc-tec/openbao-operator/issues/228)) ([b6f6848](https://github.com/dc-tec/openbao-operator/commit/b6f68487add3723932ff454f18d63f0c6688cac5))
* **vap:** harden OpenBaoRestore VAP guardrails + allow default backup executor image ([#76](https://github.com/dc-tec/openbao-operator/issues/76)) ([93524c8](https://github.com/dc-tec/openbao-operator/commit/93524c8b91563bd5bee91caf2ef0d9360d0a2b04))


### Bug Fixes

* **admission:** add admission check ([50d3af0](https://github.com/dc-tec/openbao-operator/commit/50d3af0aa06773e5ea5ee98a1194cba7c9f98b1e))
* **admission:** allow hardened image verification defaults ([#240](https://github.com/dc-tec/openbao-operator/issues/240)) ([817f144](https://github.com/dc-tec/openbao-operator/commit/817f144a066b21bf05040dd03d35e45ea37b8eb3))
* **admission:** guard hardened security context overrides ([#390](https://github.com/dc-tec/openbao-operator/issues/390)) ([d0a6533](https://github.com/dc-tec/openbao-operator/commit/d0a6533a4c5dbb7b23e4c0c83abf6ee07a5b491e))
* **admission:** implement security/rbac improvements ([95cd1b2](https://github.com/dc-tec/openbao-operator/commit/95cd1b246c2eacb18e9fa8da977a44ee7faf1313))
* **api,security:** harden CRD/admission contracts and guardrails ([#106](https://github.com/dc-tec/openbao-operator/issues/106)) ([40f49d8](https://github.com/dc-tec/openbao-operator/commit/40f49d890a757c3623f08142355fb5c1db3ad5e6))
* **api:** switch SecretReference to LocalObjectReference ([c3b8fef](https://github.com/dc-tec/openbao-operator/commit/c3b8fefd41e8f06b1b4456f66861974d06de4428))
* **auth:** harden OIDC discovery and add least-privilege RBAC + admission guardrails ([#86](https://github.com/dc-tec/openbao-operator/issues/86)) ([d128a5d](https://github.com/dc-tec/openbao-operator/commit/d128a5d653aa504bbaaadaf48dbd240fc8c7c8da))
* **auth:** harden operator OIDC bootstrap discovery ([#242](https://github.com/dc-tec/openbao-operator/issues/242)) ([c6fef5d](https://github.com/dc-tec/openbao-operator/commit/c6fef5d05860dab3de42f37cf45c9360c9723986))
* **auth:** retry kubernetes jwks discovery via api service ([#241](https://github.com/dc-tec/openbao-operator/issues/241)) ([37358f6](https://github.com/dc-tec/openbao-operator/commit/37358f65677819cd8d9ac52cd9775ebe718f23ea))
* **backup:** align retention behavior across providers and refactor backup/restore flow ([#105](https://github.com/dc-tec/openbao-operator/issues/105)) ([2e1fa9d](https://github.com/dc-tec/openbao-operator/commit/2e1fa9d941f818512155e34d6e7c8a9c6a620689))
* **backup:** make sure backup jobs are idempotent ([#47](https://github.com/dc-tec/openbao-operator/issues/47)) ([8e2ec6f](https://github.com/dc-tec/openbao-operator/commit/8e2ec6f058928a169718908b3e7fa38150ffcf80))
* **backup:** manual / scheduled backups ([f68172e](https://github.com/dc-tec/openbao-operator/commit/f68172e4800ce383d8a5b40e910465f6ad1ce86c))
* **backup:** remove unused function ([556161f](https://github.com/dc-tec/openbao-operator/commit/556161f542a71570fb94660a4d986a51df660a84))
* **backup:** upgrade paths ([e2bb9b5](https://github.com/dc-tec/openbao-operator/commit/e2bb9b5ceded236632ce89eee43a001efc0dca70))
* **bluegreen:** harden deterministic upgrade flow, tests, and docs ([#104](https://github.com/dc-tec/openbao-operator/issues/104)) ([bb64c2e](https://github.com/dc-tec/openbao-operator/commit/bb64c2ed593962f94c004971ec0986270a5270e0))
* **build:** stabilize byte reproducibility gates for checksums and sbom outputs ([#180](https://github.com/dc-tec/openbao-operator/issues/180)) ([7547ea4](https://github.com/dc-tec/openbao-operator/commit/7547ea48876ddda4788a4d004da31f5f4ea7b985))
* **chart:** sync helm chart ([9c22829](https://github.com/dc-tec/openbao-operator/commit/9c228297ace116396f351290620eb44991739d57))
* **chart:** sync helm chart ([#7](https://github.com/dc-tec/openbao-operator/issues/7)) ([507c364](https://github.com/dc-tec/openbao-operator/commit/507c36400b8f83b75e614df3fd34fcddd0e12283))
* **ci:** allow PR label sync to write labels ([#307](https://github.com/dc-tec/openbao-operator/issues/307)) ([51591d8](https://github.com/dc-tec/openbao-operator/commit/51591d8a212019134cb290d3c876385b08745e01))
* **ci:** always run perf weekly issue job after failed schedule check ([3d0eb18](https://github.com/dc-tec/openbao-operator/commit/3d0eb189ccda2545def4e3635dd5aabb8a24c599))
* **ci:** create kind cluster in release e2e gate ([#135](https://github.com/dc-tec/openbao-operator/issues/135)) ([838fe67](https://github.com/dc-tec/openbao-operator/commit/838fe6744cdde4346fe000c092c8059700de0664))
* **ci:** handle kind load failures for multi-arch OpenBao images ([#125](https://github.com/dc-tec/openbao-operator/issues/125)) ([05038ba](https://github.com/dc-tec/openbao-operator/commit/05038baaf0a706ee4c4c1c1d944f93a84c4768f0))
* **ci:** harden mainline publish workflows ([#224](https://github.com/dc-tec/openbao-operator/issues/224)) ([3bebc04](https://github.com/dc-tec/openbao-operator/commit/3bebc04970d43c77ba7fc7bcfac5cc7c63a18937))
* **ci:** replace dangerous PR labeling workflow ([#304](https://github.com/dc-tec/openbao-operator/issues/304)) ([b3740f8](https://github.com/dc-tec/openbao-operator/commit/b3740f89f65379b734ac70e8db5cd5982e479939))
* **ci:** restore security and bot PR pipeline stability ([#129](https://github.com/dc-tec/openbao-operator/issues/129)) ([ae8d297](https://github.com/dc-tec/openbao-operator/commit/ae8d297eae7ed5673d919673167ac4bdea002e1c))
* **ci:** stabilize nightly e2e image refs and matrix check naming ([#121](https://github.com/dc-tec/openbao-operator/issues/121)) ([c69993d](https://github.com/dc-tec/openbao-operator/commit/c69993d4eace0c5104aaf1659f390a25fadb4b69))
* **ci:** stabilize release/build reproducibility and align CI documentation ([#179](https://github.com/dc-tec/openbao-operator/issues/179)) ([4378cfe](https://github.com/dc-tec/openbao-operator/commit/4378cfe9cf33c35b87ea429290608a2d6a3f0c18))
* **ci:** unblock draft release lookup and run reproducibility post-release ([#185](https://github.com/dc-tec/openbao-operator/issues/185)) ([4fa1089](https://github.com/dc-tec/openbao-operator/commit/4fa10896da12c125cf7873567fd0e49876299517))
* **controller:** infer BlueImage from running pods to prevent premature upgrades ([#95](https://github.com/dc-tec/openbao-operator/issues/95)) ([dfdc11e](https://github.com/dc-tec/openbao-operator/commit/dfdc11efe964fa427b69cfebf0b22bac0fa98d3e))
* **controller:** Prevent data loss by orphaning secrets when DeletionPolicy is Retain ([#11](https://github.com/dc-tec/openbao-operator/issues/11)) ([0899cfa](https://github.com/dc-tec/openbao-operator/commit/0899cfa44d53deea6aaf65343d44b61c6a488168))
* **controller:** prevent OpenBaoCluster resourceVersion churn ([#49](https://github.com/dc-tec/openbao-operator/issues/49)) ([c0e4fe8](https://github.com/dc-tec/openbao-operator/commit/c0e4fe88c628cec4cab6ed6cd1bc053378f27d1e))
* **controller:** recheck admission dependencies at runtime ([#262](https://github.com/dc-tec/openbao-operator/issues/262)) ([8203a59](https://github.com/dc-tec/openbao-operator/commit/8203a59048f54c1b89a5862235b602cc9b0fb376))
* **controller:** refresh cluster status on standard cadence ([#257](https://github.com/dc-tec/openbao-operator/issues/257)) ([5fd50f3](https://github.com/dc-tec/openbao-operator/commit/5fd50f371870d3012c485e93b2839a7394cd272a))
* **controller:** remove force ownership of status ([#70](https://github.com/dc-tec/openbao-operator/issues/70)) ([e59e5da](https://github.com/dc-tec/openbao-operator/commit/e59e5da6d22ea82dde7c8c272447e4744991b51e))
* **controller:** timeout for image verification ([cbcd9cf](https://github.com/dc-tec/openbao-operator/commit/cbcd9cf753ee6b33d0167d8195cfff69a13e966c))
* **core:** add temporary transient error ([e0aeb21](https://github.com/dc-tec/openbao-operator/commit/e0aeb2146e9e713226c8aeecab06508d69295b2d))
* **core:** check token existence ([f4669f5](https://github.com/dc-tec/openbao-operator/commit/f4669f5b2cc2fe844e02e1282e8e3e8e12d5763a))
* **core:** harden controller determinism and idempotency  ([#107](https://github.com/dc-tec/openbao-operator/issues/107)) ([e573bf9](https://github.com/dc-tec/openbao-operator/commit/e573bf96702c4fca761c34456b9898f5d7d63e90))
* **core:** improve container status checking ([e357dcc](https://github.com/dc-tec/openbao-operator/commit/e357dcc0fd385adb4b6a400eaca3cd84ef52bcc4))
* **core:** rbac and admission hardening ([477be64](https://github.com/dc-tec/openbao-operator/commit/477be6472cd6d45324b2ec879a70d50bd10fcf2f))
* **deps:** resolve security vulnerabilities in go-tuf/v2 and rekor dependencies ([#74](https://github.com/dc-tec/openbao-operator/issues/74)) ([ecbfba8](https://github.com/dc-tec/openbao-operator/commit/ecbfba80715689bf0eb1689ec370befbfad6cd83))
* **e2e:** sentinel drift detection robustness ([648f3df](https://github.com/dc-tec/openbao-operator/commit/648f3df3b08e71633f172993a22ecaf0559acfdb))
* **e2e:** unused param ([b7a9c02](https://github.com/dc-tec/openbao-operator/commit/b7a9c0294172e5d52c048e117a874239ffb6d10a))
* **helm:** allow global values in chart schema ([#378](https://github.com/dc-tec/openbao-operator/issues/378)) ([5dad02e](https://github.com/dc-tec/openbao-operator/commit/5dad02ebc4253ddb366f636e3aea60ffce5f4ffa))
* **helm:** Helm provisioner admission identity ([#387](https://github.com/dc-tec/openbao-operator/issues/387)) ([f781c70](https://github.com/dc-tec/openbao-operator/commit/f781c70b885973b0d682cc102607d3e0b41f36dd))
* **images:** fail-fast on missing OPERATOR_VERSION environment variable ([#25](https://github.com/dc-tec/openbao-operator/issues/25)) ([1a42097](https://github.com/dc-tec/openbao-operator/commit/1a42097c8fd80bfe773682865c1119b29ca77d02))
* Implement versioned default images for backup, upgrade, and init container ([#14](https://github.com/dc-tec/openbao-operator/issues/14)) ([1b34f78](https://github.com/dc-tec/openbao-operator/commit/1b34f785009750a2667293d31334260fee04716d))
* **infra:** add IPv6/dual-stack support for listener binding and development egress rules ([#56](https://github.com/dc-tec/openbao-operator/issues/56)) ([7bfdb41](https://github.com/dc-tec/openbao-operator/commit/7bfdb41840bed338cbfcede82be3aea6642a7a53))
* **infra:** delete scaled-down raft PVCs ([#341](https://github.com/dc-tec/openbao-operator/issues/341)) ([f406e90](https://github.com/dc-tec/openbao-operator/commit/f406e9029d94c8e7984d77b66cf02b8a97f3c339))
* **infra:** exclude job pods from pdb ([#9](https://github.com/dc-tec/openbao-operator/issues/9)) ([825a191](https://github.com/dc-tec/openbao-operator/commit/825a1916d68a6a0bb09c4f46c1251cf2af9cd159))
* **infra:** fail closed on hostile OIDC bootstrap discovery ([#263](https://github.com/dc-tec/openbao-operator/issues/263)) ([2dbd9be](https://github.com/dc-tec/openbao-operator/commit/2dbd9be4a01395d071af79876ef9cc9989cf606c))
* **infra:** improve initialization robustness by treating transient Secret/RBAC errors as retriable and hardening root-token creation ([#55](https://github.com/dc-tec/openbao-operator/issues/55)) ([f760ac5](https://github.com/dc-tec/openbao-operator/commit/f760ac5c17bd99f747e8c3dc637bdcee1b4cb511))
* **infra:** resolve BackendTLSPolicy mismatch and cleanup stale services after Blue/Green upgrade ([#10](https://github.com/dc-tec/openbao-operator/issues/10)) ([7052a54](https://github.com/dc-tec/openbao-operator/commit/7052a54145a4d9ac1a1d9ed3b7fdb1cc8de994a2))
* **infra:** stop apiserver endpoint autodetection; use service VIP allow-list with optional endpoint IPs ([#54](https://github.com/dc-tec/openbao-operator/issues/54)) ([d73179a](https://github.com/dc-tec/openbao-operator/commit/d73179a434428bb787684791d1de88dc778f138f))
* **init:** retrty writing root token to secret to handle transient cr… ([#84](https://github.com/dc-tec/openbao-operator/issues/84)) ([e100176](https://github.com/dc-tec/openbao-operator/commit/e1001769b05fbccae2c861b586dd3eac3eaefd8c))
* **kube:** add job check ([a7439a9](https://github.com/dc-tec/openbao-operator/commit/a7439a9fe060a4710deda76bea6b7bfafde18020))
* **manifests:** make JWT auth bootstrap a opt-in feature ([ded02a3](https://github.com/dc-tec/openbao-operator/commit/ded02a3173c672e7cbc03f5e993635e1cb345663))
* **manifests:** secure defaults and profiles ([6617383](https://github.com/dc-tec/openbao-operator/commit/66173839968834008119e07cf29cc99188ef8121))
* **multitenancy:** gate cluster reconcile on tenant onboarding ([#359](https://github.com/dc-tec/openbao-operator/issues/359)) ([cfd850f](https://github.com/dc-tec/openbao-operator/commit/cfd850fcf819c4d1562644cc9495143cfee69b27))
* **network:** Require source-scoped managed Ingress access ([#389](https://github.com/dc-tec/openbao-operator/issues/389)) ([a3cec85](https://github.com/dc-tec/openbao-operator/commit/a3cec85a56230560be8196ac02666ad38b7e136d))
* **nightly:** harden init token persistence and e2e autopilot reliability ([#117](https://github.com/dc-tec/openbao-operator/issues/117)) ([f85886f](https://github.com/dc-tec/openbao-operator/commit/f85886fc92b5df3eff30b5075659b41279e8717d))
* **openbao:** handle 403 forbidden gracefully ([#94](https://github.com/dc-tec/openbao-operator/issues/94)) ([4243f67](https://github.com/dc-tec/openbao-operator/commit/4243f67d68e69d8406b5e0702c806a4f876dd774))
* **openbao:** stage safe raft scale-downs ([#339](https://github.com/dc-tec/openbao-operator/issues/339)) ([4da1ec7](https://github.com/dc-tec/openbao-operator/commit/4da1ec74f8e4e45e710a0fae51f86bbf44c257c8))
* **probe:** stabilize openbao workload probes ([#371](https://github.com/dc-tec/openbao-operator/issues/371)) ([260547b](https://github.com/dc-tec/openbao-operator/commit/260547b71d3e12e2ec97ae500f9ed63ab1619804))
* **provisioner:** reduce release reconciliation log noise ([#370](https://github.com/dc-tec/openbao-operator/issues/370)) ([b2f2bca](https://github.com/dc-tec/openbao-operator/commit/b2f2bcaf18dfef15348aa02b9f3de224c02e38ab))
* **release-0.2:** backport 0.2.1 fixes ([069d9b4](https://github.com/dc-tec/openbao-operator/commit/069d9b454e95dda6c00788cc9878590a30e1146a))
* **release:** grant tag workflow comment permissions ([#295](https://github.com/dc-tec/openbao-operator/issues/295)) ([61ec413](https://github.com/dc-tec/openbao-operator/commit/61ec413d7b640e446d135e67e98bbc17c85badec))
* **release:** remove unsupported tag app scope ([#296](https://github.com/dc-tec/openbao-operator/issues/296)) ([e794a76](https://github.com/dc-tec/openbao-operator/commit/e794a7629f3ad31083834a7d5b0f63d64cc4b93e))
* **release:** sign release tags and trim release gates ([#298](https://github.com/dc-tec/openbao-operator/issues/298)) ([33a687b](https://github.com/dc-tec/openbao-operator/commit/33a687b9b93537bffd944791d7f02fc7d48fe855))
* **rolling:** handle retry status conflicts during upgrade resume ([#192](https://github.com/dc-tec/openbao-operator/issues/192)) ([c6957f2](https://github.com/dc-tec/openbao-operator/commit/c6957f280e1264b7912d0304d5937d6227b8a5f2))
* **security;e2e:** verify signed hardened/acme flows in CI/nightly and support digest-safe keyless defaults ([#116](https://github.com/dc-tec/openbao-operator/issues/116)) ([3b966fe](https://github.com/dc-tec/openbao-operator/commit/3b966fe25097fbb4e490682f93bc8671463741f2))
* **security:** fail closed for configured trusted roots ([#393](https://github.com/dc-tec/openbao-operator/issues/393)) ([04cbd64](https://github.com/dc-tec/openbao-operator/commit/04cbd64cf0356f111f0e3c0450b859008e6c5b69))
* **security:** harden managed image digests and gateway validation reads ([#243](https://github.com/dc-tec/openbao-operator/issues/243)) ([62a44d0](https://github.com/dc-tec/openbao-operator/commit/62a44d006fc27019e2f5cc1fa58ddb216e088503))
* **security:** implement image verification LRU cache; docker auth handeling ([#18](https://github.com/dc-tec/openbao-operator/issues/18)) ([a4b7203](https://github.com/dc-tec/openbao-operator/commit/a4b720313ec7fa40a7b0123de4bbbbe090441c0e))
* **security:** performance issue image verification by reording cache lookups ([#12](https://github.com/dc-tec/openbao-operator/issues/12)) ([a5ca5eb](https://github.com/dc-tec/openbao-operator/commit/a5ca5eb1268d9afe98d8bcc0ce6c3dda0efde20c))
* **security:** remove resolved govulncheck ignores ([#249](https://github.com/dc-tec/openbao-operator/issues/249)) ([58be543](https://github.com/dc-tec/openbao-operator/commit/58be543c57c0b47b977271d1e51eb0baa49853f9))
* **security:** validate UMASK bounds in bao-wrapper ([#195](https://github.com/dc-tec/openbao-operator/issues/195)) ([08b5f8a](https://github.com/dc-tec/openbao-operator/commit/08b5f8a6a92325d176ba40e3c79a4106570ab029))
* **security:** wrap bundle fallback verification error ([#200](https://github.com/dc-tec/openbao-operator/issues/200)) ([827899e](https://github.com/dc-tec/openbao-operator/commit/827899ea077c149e93ccf7aaf3c9d333a45b37c5))
* **sentinel:** prevent noisy neighbors and thundering herd behavior ([57eb7bd](https://github.com/dc-tec/openbao-operator/commit/57eb7bdfd9b714e2d64c0954d5a36c260dde7efa))
* **sentinel:** rely on uuids instead of timestamps as sentinel triggerid ([#6](https://github.com/dc-tec/openbao-operator/issues/6)) ([f88b697](https://github.com/dc-tec/openbao-operator/commit/f88b697f6dc13f19cf9a00d2764a4ed0be58868d))
* **status:** make lifecycle status guidance more actionable ([#227](https://github.com/dc-tec/openbao-operator/issues/227)) ([6bf9147](https://github.com/dc-tec/openbao-operator/commit/6bf9147aa42231f0f2494c00f6c9d77924a7e292))
* **status:** mark unsafe admission mode not production-ready ([#391](https://github.com/dc-tec/openbao-operator/issues/391)) ([98022a3](https://github.com/dc-tec/openbao-operator/commit/98022a3925742e011dbb8ce1fb55c2c79c5a1496))
* **storage:** enforce storage class immutability consistently ([#215](https://github.com/dc-tec/openbao-operator/issues/215)) ([c0a551f](https://github.com/dc-tec/openbao-operator/commit/c0a551fd8e5e0c653d151de5b17990573767c333))
* **upgrade:** add metrics for upgrade ([936d71e](https://github.com/dc-tec/openbao-operator/commit/936d71edca1f111a40c8a04bd32910459c24fc93))
* **upgrade:** clear rolling retry failure state with merge status patch ([#205](https://github.com/dc-tec/openbao-operator/issues/205)) ([f4b47f9](https://github.com/dc-tec/openbao-operator/commit/f4b47f9403fdd1ea954dd7af902d194f7889b055))
* **upgrade:** complete SSA ownership migration ([#345](https://github.com/dc-tec/openbao-operator/issues/345)) ([eafa931](https://github.com/dc-tec/openbao-operator/commit/eafa9317acf33155cc7863924b5cb4a8725f97bc))
* **upgrade:** harden bluegreen and rolling recovery flakes ([#374](https://github.com/dc-tec/openbao-operator/issues/374)) ([62cf706](https://github.com/dc-tec/openbao-operator/commit/62cf706df50b8ff462e5893166fc61b83749b298))
* **upgrade:** harden OpenBaoCluster upgrade validation, recovery, and documentation ([#225](https://github.com/dc-tec/openbao-operator/issues/225)) ([a170c0a](https://github.com/dc-tec/openbao-operator/commit/a170c0acb3c835016f32483169d3c61e07ab26b3))
* **upgrade:** improve upgrade manager stability ([#13](https://github.com/dc-tec/openbao-operator/issues/13)) ([c6a1b34](https://github.com/dc-tec/openbao-operator/commit/c6a1b34a515e7ed4201d61cd2b564ba2b0a9b5bf))
* **upgrade:** make rolling upgrades deterministic and harden rolling upgrade coverage ([#103](https://github.com/dc-tec/openbao-operator/issues/103)) ([5f3edfd](https://github.com/dc-tec/openbao-operator/commit/5f3edfd3d1b111b3b07a8818aa743f523ab8d810))
* **upgrade:** revert partition update to MergeFrom to fix StatefulSet validation ([#52](https://github.com/dc-tec/openbao-operator/issues/52)) ([504c319](https://github.com/dc-tec/openbao-operator/commit/504c31970030519ed602f16ebc3d7be5b339d32c))
* **upgrade:** set executor job resource requirements ([#392](https://github.com/dc-tec/openbao-operator/issues/392)) ([8efb8da](https://github.com/dc-tec/openbao-operator/commit/8efb8da900d378139e35bd32c54489bcc74bec15))
* **upgrade:** treat raft promote already-voter as no-op ([#382](https://github.com/dc-tec/openbao-operator/issues/382)) ([7d25753](https://github.com/dc-tec/openbao-operator/commit/7d25753b9c5c780e174e8adb5487f48c67128267))
* **upgrade:** use SSA for upgrade manager ([d0c289c](https://github.com/dc-tec/openbao-operator/commit/d0c289ce76686f7329e79cbbbfdc29b172446c74))
* **upgrade:** verify default helper images for hardened clusters ([#308](https://github.com/dc-tec/openbao-operator/issues/308)) ([8bfeabb](https://github.com/dc-tec/openbao-operator/commit/8bfeabb6b79a8d897617b0aac63d89be9530ef16))
* **validation:** block upgrade strategy switches ([#288](https://github.com/dc-tec/openbao-operator/issues/288)) ([b5f0af4](https://github.com/dc-tec/openbao-operator/commit/b5f0af4a7e5c7fbceb733a52e4bc3327171f93c6))
* **vap:** require self init requests when self initialization is enabled ([#82](https://github.com/dc-tec/openbao-operator/issues/82)) ([c572aaa](https://github.com/dc-tec/openbao-operator/commit/c572aaa392ecc8c8f6dccdee5203a964055a6106))
* **vap:** stuck Job deletions by allowing GC Job-finalizer updates in lock-managed-resource-mutations VAP ([#53](https://github.com/dc-tec/openbao-operator/issues/53)) ([0c56a87](https://github.com/dc-tec/openbao-operator/commit/0c56a8726c3a972566fc4a93b8a8d3d9bbd99ae7))


### Miscellaneous Chores

* **release:** release 0.1.0 ([#302](https://github.com/dc-tec/openbao-operator/issues/302)) ([ebcaf03](https://github.com/dc-tec/openbao-operator/commit/ebcaf03b7ca60a02d56e64135a45e6f1e20be424))
* **release:** release 0.1.0-rc.7 ([#299](https://github.com/dc-tec/openbao-operator/issues/299)) ([f1aa990](https://github.com/dc-tec/openbao-operator/commit/f1aa990e7ac08d4cf203d61ede7fd8b3448419bd))
* **release:** set release target to 0.1.0-rc.1 ([#133](https://github.com/dc-tec/openbao-operator/issues/133)) ([ad509ed](https://github.com/dc-tec/openbao-operator/commit/ad509edfa50936cc8b263fcae1d1233fa6b9f47b))
* **release:** set release target to 0.1.0-rc.2 ([#136](https://github.com/dc-tec/openbao-operator/issues/136)) ([624238d](https://github.com/dc-tec/openbao-operator/commit/624238df4f561709ce0390f3332c0737685d7a67))
* **release:** set release target to 0.1.0-rc.3 ([#176](https://github.com/dc-tec/openbao-operator/issues/176)) ([af6043e](https://github.com/dc-tec/openbao-operator/commit/af6043ee5c02d6440b9de9401ce8bb9c332831ba))
* **release:** set release target to 0.1.0-rc.4 ([#183](https://github.com/dc-tec/openbao-operator/issues/183)) ([b5402ea](https://github.com/dc-tec/openbao-operator/commit/b5402eaed71cf776dfa6b6a42b23c5030b38896c))
* **release:** set release target to 0.1.0-rc.5 ([#187](https://github.com/dc-tec/openbao-operator/issues/187)) ([39649ee](https://github.com/dc-tec/openbao-operator/commit/39649ee68ef28ed3c94cfebf2dc9de04f3ff2466))
* trigger release-please for 0.1.0-rc.6 ([#293](https://github.com/dc-tec/openbao-operator/issues/293)) ([9f8bfa1](https://github.com/dc-tec/openbao-operator/commit/9f8bfa193a8bb45d3327f99a6e365e49cab9879c))


### Code Refactoring

* **config:** openbaocluster config renderer ([a230262](https://github.com/dc-tec/openbao-operator/commit/a230262c4795566c21ad58a65b74364e7cdd36b6))
* **controller:** openbaocluster refactor; sentinel improvements ([9d0de98](https://github.com/dc-tec/openbao-operator/commit/9d0de984d9681d53f4c5569ff84443ae46e2bad5))
* **core:** remove Sentinel drift detection (VAP hardening) ([#39](https://github.com/dc-tec/openbao-operator/issues/39)) ([d289cf2](https://github.com/dc-tec/openbao-operator/commit/d289cf262213ab13ca3c9e3631df1d4845ee6fc7))
* **upgrade:** simplify blue/green cutover and split rolling strategy ([#37](https://github.com/dc-tec/openbao-operator/issues/37)) ([7453e23](https://github.com/dc-tec/openbao-operator/commit/7453e23880b1edbfa0c825d6982c29893d4ac08d))
* **upgrade:** upgrade manager; blue/green upgrades ([2ba56a4](https://github.com/dc-tec/openbao-operator/commit/2ba56a426caa12a79a069700b0b2a4ede44156e1))

## [0.2.1](https://github.com/dc-tec/openbao-operator/compare/0.2.0...0.2.1) (2026-05-18)


### Bug Fixes

* **release-0.2:** backport 0.2.1 fixes ([069d9b4](https://github.com/dc-tec/openbao-operator/commit/069d9b454e95dda6c00788cc9878590a30e1146a))

## [0.2.0](https://github.com/dc-tec/openbao-operator/compare/0.1.0...0.2.0) (2026-05-18)


### Features

* **admission:** authorize maintenance through RBAC ([#347](https://github.com/dc-tec/openbao-operator/issues/347)) ([b7c05a7](https://github.com/dc-tec/openbao-operator/commit/b7c05a770bcc97ea1931caf0a3c05919540c38ab))
* **api:** add runtime restart controls ([#348](https://github.com/dc-tec/openbao-operator/issues/348)) ([b1efd34](https://github.com/dc-tec/openbao-operator/commit/b1efd3442c2c5cd0a58c654b749103ab7cf5ac81))
* **readreplicas:** add steady-state read replica topology and status ([#361](https://github.com/dc-tec/openbao-operator/issues/361)) ([9a74c14](https://github.com/dc-tec/openbao-operator/commit/9a74c143e9061f42f5c7557af7a7e9b767252926))
* **readreplicas:** integrate read replicas with upgrade and restore workflows ([#362](https://github.com/dc-tec/openbao-operator/issues/362)) ([e8bf8b8](https://github.com/dc-tec/openbao-operator/commit/e8bf8b820c06ccab1fb81a9df25223dfbf4e0666))


### Bug Fixes

* **admission:** guard hardened security context overrides ([#390](https://github.com/dc-tec/openbao-operator/issues/390)) ([d0a6533](https://github.com/dc-tec/openbao-operator/commit/d0a6533a4c5dbb7b23e4c0c83abf6ee07a5b491e))
* **ci:** allow PR label sync to write labels ([#307](https://github.com/dc-tec/openbao-operator/issues/307)) ([51591d8](https://github.com/dc-tec/openbao-operator/commit/51591d8a212019134cb290d3c876385b08745e01))
* **ci:** replace dangerous PR labeling workflow ([#304](https://github.com/dc-tec/openbao-operator/issues/304)) ([b3740f8](https://github.com/dc-tec/openbao-operator/commit/b3740f89f65379b734ac70e8db5cd5982e479939))
* **helm:** allow global values in chart schema ([#378](https://github.com/dc-tec/openbao-operator/issues/378)) ([5dad02e](https://github.com/dc-tec/openbao-operator/commit/5dad02ebc4253ddb366f636e3aea60ffce5f4ffa))
* **helm:** Helm provisioner admission identity ([#387](https://github.com/dc-tec/openbao-operator/issues/387)) ([f781c70](https://github.com/dc-tec/openbao-operator/commit/f781c70b885973b0d682cc102607d3e0b41f36dd))
* **infra:** delete scaled-down raft PVCs ([#341](https://github.com/dc-tec/openbao-operator/issues/341)) ([f406e90](https://github.com/dc-tec/openbao-operator/commit/f406e9029d94c8e7984d77b66cf02b8a97f3c339))
* **multitenancy:** gate cluster reconcile on tenant onboarding ([#359](https://github.com/dc-tec/openbao-operator/issues/359)) ([cfd850f](https://github.com/dc-tec/openbao-operator/commit/cfd850fcf819c4d1562644cc9495143cfee69b27))
* **network:** Require source-scoped managed Ingress access ([#389](https://github.com/dc-tec/openbao-operator/issues/389)) ([a3cec85](https://github.com/dc-tec/openbao-operator/commit/a3cec85a56230560be8196ac02666ad38b7e136d))
* **openbao:** stage safe raft scale-downs ([#339](https://github.com/dc-tec/openbao-operator/issues/339)) ([4da1ec7](https://github.com/dc-tec/openbao-operator/commit/4da1ec74f8e4e45e710a0fae51f86bbf44c257c8))
* **probe:** stabilize openbao workload probes ([#371](https://github.com/dc-tec/openbao-operator/issues/371)) ([260547b](https://github.com/dc-tec/openbao-operator/commit/260547b71d3e12e2ec97ae500f9ed63ab1619804))
* **provisioner:** reduce release reconciliation log noise ([#370](https://github.com/dc-tec/openbao-operator/issues/370)) ([b2f2bca](https://github.com/dc-tec/openbao-operator/commit/b2f2bcaf18dfef15348aa02b9f3de224c02e38ab))
* **release-0.2:** backport 0.2.1 fixes ([069d9b4](https://github.com/dc-tec/openbao-operator/commit/069d9b454e95dda6c00788cc9878590a30e1146a))
* **security:** fail closed for configured trusted roots ([#393](https://github.com/dc-tec/openbao-operator/issues/393)) ([04cbd64](https://github.com/dc-tec/openbao-operator/commit/04cbd64cf0356f111f0e3c0450b859008e6c5b69))
* **status:** mark unsafe admission mode not production-ready ([#391](https://github.com/dc-tec/openbao-operator/issues/391)) ([98022a3](https://github.com/dc-tec/openbao-operator/commit/98022a3925742e011dbb8ce1fb55c2c79c5a1496))
* **upgrade:** complete SSA ownership migration ([#345](https://github.com/dc-tec/openbao-operator/issues/345)) ([eafa931](https://github.com/dc-tec/openbao-operator/commit/eafa9317acf33155cc7863924b5cb4a8725f97bc))
* **upgrade:** harden bluegreen and rolling recovery flakes ([#374](https://github.com/dc-tec/openbao-operator/issues/374)) ([62cf706](https://github.com/dc-tec/openbao-operator/commit/62cf706df50b8ff462e5893166fc61b83749b298))
* **upgrade:** set executor job resource requirements ([#392](https://github.com/dc-tec/openbao-operator/issues/392)) ([8efb8da](https://github.com/dc-tec/openbao-operator/commit/8efb8da900d378139e35bd32c54489bcc74bec15))
* **upgrade:** treat raft promote already-voter as no-op ([#382](https://github.com/dc-tec/openbao-operator/issues/382)) ([7d25753](https://github.com/dc-tec/openbao-operator/commit/7d25753b9c5c780e174e8adb5487f48c67128267))
* **upgrade:** verify default helper images for hardened clusters ([#308](https://github.com/dc-tec/openbao-operator/issues/308)) ([8bfeabb](https://github.com/dc-tec/openbao-operator/commit/8bfeabb6b79a8d897617b0aac63d89be9530ef16))

## [0.1.0](https://github.com/dc-tec/openbao-operator/compare/0.2.1...0.1.0) (2026-05-18)


### ⚠ BREAKING CHANGES

* **core:** Improve OIDC/JWT bootstrap, update strategy configuration and configuration ergonomics ([#73](https://github.com/dc-tec/openbao-operator/issues/73))
* **core:** remove Sentinel drift detection (VAP hardening) ([#39](https://github.com/dc-tec/openbao-operator/issues/39))
* **upgrade:** simplify blue/green cutover and split rolling strategy ([#37](https://github.com/dc-tec/openbao-operator/issues/37))
* **config:** openbaocluster config renderer
* **upgrade:** upgrade manager; blue/green upgrades
* **controller:** openbaocluster refactor; sentinel improvements

### Features

* **admission:** authorize maintenance through RBAC ([#347](https://github.com/dc-tec/openbao-operator/issues/347)) ([b7c05a7](https://github.com/dc-tec/openbao-operator/commit/b7c05a770bcc97ea1931caf0a3c05919540c38ab))
* **api:** add OpenBaoCluster observedGeneration and printer columns ([#286](https://github.com/dc-tec/openbao-operator/issues/286)) ([1c8f8ae](https://github.com/dc-tec/openbao-operator/commit/1c8f8aeb143fd90ca6452d2f72852c47b14ab5ea))
* **api:** add runtime restart controls ([#348](https://github.com/dc-tec/openbao-operator/issues/348)) ([b1efd34](https://github.com/dc-tec/openbao-operator/commit/b1efd3442c2c5cd0a58c654b749103ab7cf5ac81))
* **api:** improve sentinel observability ([b9d4168](https://github.com/dc-tec/openbao-operator/commit/b9d41686964165291a974d900d9050d8be8983c0))
* **ast-grep:** add policy-driven architecture guardrails with CI enforcement ([#201](https://github.com/dc-tec/openbao-operator/issues/201)) ([1faee9a](https://github.com/dc-tec/openbao-operator/commit/1faee9a6b000d0e68770d7e2894e68d66f13f534))
* **backup;restore:** azure blob storage and GCS support as backup provider ([#71](https://github.com/dc-tec/openbao-operator/issues/71)) ([e8a2f2d](https://github.com/dc-tec/openbao-operator/commit/e8a2f2dd68b4af96136d0e387e9199e934a74c82))
* **bluegreen:** blue/green traffic switching improvements ([5e5f815](https://github.com/dc-tec/openbao-operator/commit/5e5f8157e52dd7dfcacd07565cd35270c0ec3f20))
* **charts:** operator helm chart ([c00ff58](https://github.com/dc-tec/openbao-operator/commit/c00ff58ab1d39b64919acad5456ae221c8b69fc1))
* **config:** structured config for self-init ([abf2259](https://github.com/dc-tec/openbao-operator/commit/abf22590241d1a559bdba857440f0918760a78a4))
* **controller;chart;rbac:** controller hardening, Helm sync automation, and RBAC race fix ([#40](https://github.com/dc-tec/openbao-operator/issues/40)) ([c9dd0b5](https://github.com/dc-tec/openbao-operator/commit/c9dd0b54857a60d2dfe47bcc10d4a75929412a27))
* **controller:** add extra metrics ([3ed3915](https://github.com/dc-tec/openbao-operator/commit/3ed3915ad5d37349891bbc0abadccca7ce0b0643))
* **controller:** improve event filtering using centralized predicates ([968df6c](https://github.com/dc-tec/openbao-operator/commit/968df6c7c58cd7fb95793208605c4ae2f8fe4e8d))
* **controller:** single tenancy support ([49b7327](https://github.com/dc-tec/openbao-operator/commit/49b7327caed9394e89999023a4cd1f2488faf2a4))
* **core:** add consistent Kubernetes lifecycle events ([#226](https://github.com/dc-tec/openbao-operator/issues/226)) ([93687af](https://github.com/dc-tec/openbao-operator/commit/93687af087760053b01de76dc6a050e3f5c9e280))
* **core:** add perf baseline harness and gates ([#118](https://github.com/dc-tec/openbao-operator/issues/118)) ([bf91ce2](https://github.com/dc-tec/openbao-operator/commit/bf91ce24ec1de79cb96b1d1a1370938b62195dd7))
* **core:** blue/green upgrades ([1a6783e](https://github.com/dc-tec/openbao-operator/commit/1a6783eeb1cb45933d5cc146c81644fca26ccc11))
* **core:** cluster lifecycle hardening; e2e suite refactor ([#72](https://github.com/dc-tec/openbao-operator/issues/72)) ([3de5142](https://github.com/dc-tec/openbao-operator/commit/3de5142367e0076a169f1ebb14497c150dbf5722))
* **core:** enable Raft Autopilot for automatic dead server cleanup ([#44](https://github.com/dc-tec/openbao-operator/issues/44)) ([61aa711](https://github.com/dc-tec/openbao-operator/commit/61aa7115390c8cd9143f9fd4f985414c2756b909))
* **core:** harden lifecycle contracts and supporting coverage ([#237](https://github.com/dc-tec/openbao-operator/issues/237)) ([44de947](https://github.com/dc-tec/openbao-operator/commit/44de94790ed765a8eb4036490858139b6a8561bd))
* **core:** helm manifest values and templates ([6060fbd](https://github.com/dc-tec/openbao-operator/commit/6060fbd04cfb36caadd97718f604fee4250f43e3))
* **core:** Improve OIDC/JWT bootstrap, update strategy configuration and configuration ergonomics ([#73](https://github.com/dc-tec/openbao-operator/issues/73)) ([446e494](https://github.com/dc-tec/openbao-operator/commit/446e4949febbb3155aa999b2d53a720f971e8db5))
* **core:** introduce restore CRD ([4d19b72](https://github.com/dc-tec/openbao-operator/commit/4d19b72b5c74b337b61776f58f0d8f6ff711e8a9))
* **core:** introduce structured error types ([0b17ae1](https://github.com/dc-tec/openbao-operator/commit/0b17ae13e63ac49ac33d066df146ddb3190c6c40))
* **core:** make JWT audience configurable and plumb JWT bootstrap config across backup/upgrade/restore ([#57](https://github.com/dc-tec/openbao-operator/issues/57)) ([3057c61](https://github.com/dc-tec/openbao-operator/commit/3057c61293920b718d3dd5ece951858b77f5b1c6))
* **core:** OpenShift compatibility support ([#62](https://github.com/dc-tec/openbao-operator/issues/62)) ([47d7770](https://github.com/dc-tec/openbao-operator/commit/47d7770854a52d3113294ecc9cd667d8b54acd77))
* **infra;controller:** implement support for online PVC expansion of running OpenBao Clusters ([#75](https://github.com/dc-tec/openbao-operator/issues/75)) ([42fabd3](https://github.com/dc-tec/openbao-operator/commit/42fabd30c6ef0d5ec4ababe85f74fc8d37cc1810))
* **infra:** add default node and zone spreading for OpenBao StatefulSets ([#214](https://github.com/dc-tec/openbao-operator/issues/214)) ([1d7afc8](https://github.com/dc-tec/openbao-operator/commit/1d7afc8d55ddede8b24207274101a13d4352e98a))
* **infra:** add pod metadata hooks for workload identity ([#216](https://github.com/dc-tec/openbao-operator/issues/216)) ([9bd2546](https://github.com/dc-tec/openbao-operator/commit/9bd2546ccf5caf0024263073635a2a87ad6713c1))
* **infra:** Expose listenerName field for Gateway API HTTPRoute targeting ([#30](https://github.com/dc-tec/openbao-operator/issues/30)) ([5babd3f](https://github.com/dc-tec/openbao-operator/commit/5babd3f8a2b44c8135b8c1e2ea75a31062bc42e9))
* **infra:** improve hardened and ACME deployments ([#63](https://github.com/dc-tec/openbao-operator/issues/63)) ([d40600e](https://github.com/dc-tec/openbao-operator/commit/d40600effacb689a89c0f52aee1f74e74129117e))
* **infra:** make DNS namespace configurable in NetworkPolicies ([#58](https://github.com/dc-tec/openbao-operator/issues/58)) ([a675dfa](https://github.com/dc-tec/openbao-operator/commit/a675dfad6c52c030e7c265ebf60836b976957d26))
* **manifests:** install manifest ([ffc63c6](https://github.com/dc-tec/openbao-operator/commit/ffc63c669bd13c930c8e8f11ce465298e4ab4c0d))
* **manifests:** optional sentinel deployment for quicker reconcile ([081a17a](https://github.com/dc-tec/openbao-operator/commit/081a17a02060db9fb188477620fbd76d1c55522e))
* **manifests:** self-service tenant onboarding ([2a8d4d0](https://github.com/dc-tec/openbao-operator/commit/2a8d4d03bfdd53b86af93dbf4b6b4be9c9fcc9a7))
* **manifests:** structured configuration ([503961d](https://github.com/dc-tec/openbao-operator/commit/503961d790f996e1de193b321d005d2d8dcc0d4d))
* **manifests:** wire-in image verification for all components ([d94d1f9](https://github.com/dc-tec/openbao-operator/commit/d94d1f9d14c81bd994124fc964e69787045fb646))
* **observability:** add metrics, dashboards, e2e assertions; upgrade stability ([#101](https://github.com/dc-tec/openbao-operator/issues/101)) ([d4ce07d](https://github.com/dc-tec/openbao-operator/commit/d4ce07dc4d895381066ca86962fc5758f66dfd33))
* **operator:** add supported single-tenant custom identity install paths ([#239](https://github.com/dc-tec/openbao-operator/issues/239)) ([d41ff74](https://github.com/dc-tec/openbao-operator/commit/d41ff74b33133bd05bb2b2a7dadcaf4e4fe3305a))
* **perf:** refresh kind performance baseline ([#120](https://github.com/dc-tec/openbao-operator/issues/120)) ([69e5366](https://github.com/dc-tec/openbao-operator/commit/69e5366651ac500925336358fb013c0e9650e4f2))
* **policy:** enforce Hardened profile requires replicas &gt;= 3 via VAP ([#23](https://github.com/dc-tec/openbao-operator/issues/23)) ([c15ab9f](https://github.com/dc-tec/openbao-operator/commit/c15ab9fd1421b613e138861a962f51cd76b721b3))
* **provisioner:** configurable tenant resource quotas ([#50](https://github.com/dc-tec/openbao-operator/issues/50)) ([4c6fc29](https://github.com/dc-tec/openbao-operator/commit/4c6fc2915cb821547129a6c9b8e1ed73e42fd500))
* **readreplicas:** add steady-state read replica topology and status ([#361](https://github.com/dc-tec/openbao-operator/issues/361)) ([9a74c14](https://github.com/dc-tec/openbao-operator/commit/9a74c143e9061f42f5c7557af7a7e9b767252926))
* **readreplicas:** integrate read replicas with upgrade and restore workflows ([#362](https://github.com/dc-tec/openbao-operator/issues/362)) ([e8bf8b8](https://github.com/dc-tec/openbao-operator/commit/e8bf8b820c06ccab1fb81a9df25223dfbf4e0666))
* **restore:** add RBAC for restore jobs and validate authentication ([#16](https://github.com/dc-tec/openbao-operator/issues/16)) ([e7772a1](https://github.com/dc-tec/openbao-operator/commit/e7772a146482c9626c545bddff185b9a2f687c1b))
* **security:** Add admission-time protections for SSRF, TLS secrets, and tenant self-service ([#51](https://github.com/dc-tec/openbao-operator/issues/51)) ([ae2f86c](https://github.com/dc-tec/openbao-operator/commit/ae2f86c851b1369676cee536b37dd934c8ef0d0a))
* **security:** add operatorimageVerification field to CRD to allow separate verification of both OpenBao and Operator images ([#8](https://github.com/dc-tec/openbao-operator/issues/8)) ([4c1b8cc](https://github.com/dc-tec/openbao-operator/commit/4c1b8cccd1d2c47618c29efa3d08c54535da421c))
* **security:** expand control-plane audit coverage for startup, operations, and RBAC mutations ([#109](https://github.com/dc-tec/openbao-operator/issues/109)) ([b32dc97](https://github.com/dc-tec/openbao-operator/commit/b32dc97175999aadb84cecf867395a7cca2a6f85))
* **security:** harden image verification and align edge/nightly signed manifest streams ([#112](https://github.com/dc-tec/openbao-operator/issues/112)) ([b755ca3](https://github.com/dc-tec/openbao-operator/commit/b755ca333c4e598cf5904b9e68817ac540393cc5))
* **security:** harden image verification defaults and sign edge/nightly images ([#111](https://github.com/dc-tec/openbao-operator/issues/111)) ([5ffed83](https://github.com/dc-tec/openbao-operator/commit/5ffed83ea179fe14fedba50320425d8e4ce0b30c))
* **security:** harden operator RBAC with ValidatingAdmissionPolicy guardrails ([#100](https://github.com/dc-tec/openbao-operator/issues/100)) ([643fd94](https://github.com/dc-tec/openbao-operator/commit/643fd94af7f0a128bf4f62fa073ffa70ec92af18))
* **security:** tighten operator security and authentication contracts ([#238](https://github.com/dc-tec/openbao-operator/issues/238)) ([7b14fb1](https://github.com/dc-tec/openbao-operator/commit/7b14fb1cc9046cd469451c3d1d8bb4cb0cbb0302))
* **upgrade:** harden backup and restore flows ([cb542ab](https://github.com/dc-tec/openbao-operator/commit/cb542ab466e29ddbbf61460ebd9368891aa9e359))
* **upgrade:** improve upgrade manager stability by using SSA for status updates and make pre-upgrade backup job names deterministic ([#17](https://github.com/dc-tec/openbao-operator/issues/17)) ([78f6124](https://github.com/dc-tec/openbao-operator/commit/78f6124b7e3545149b86a167165fb081b7c810ac))
* **upgrade:** unify manual upgrade requests on OpenBaoCluster ([#228](https://github.com/dc-tec/openbao-operator/issues/228)) ([b6f6848](https://github.com/dc-tec/openbao-operator/commit/b6f68487add3723932ff454f18d63f0c6688cac5))
* **vap:** harden OpenBaoRestore VAP guardrails + allow default backup executor image ([#76](https://github.com/dc-tec/openbao-operator/issues/76)) ([93524c8](https://github.com/dc-tec/openbao-operator/commit/93524c8b91563bd5bee91caf2ef0d9360d0a2b04))


### Bug Fixes

* **admission:** add admission check ([50d3af0](https://github.com/dc-tec/openbao-operator/commit/50d3af0aa06773e5ea5ee98a1194cba7c9f98b1e))
* **admission:** allow hardened image verification defaults ([#240](https://github.com/dc-tec/openbao-operator/issues/240)) ([817f144](https://github.com/dc-tec/openbao-operator/commit/817f144a066b21bf05040dd03d35e45ea37b8eb3))
* **admission:** guard hardened security context overrides ([#390](https://github.com/dc-tec/openbao-operator/issues/390)) ([d0a6533](https://github.com/dc-tec/openbao-operator/commit/d0a6533a4c5dbb7b23e4c0c83abf6ee07a5b491e))
* **admission:** implement security/rbac improvements ([95cd1b2](https://github.com/dc-tec/openbao-operator/commit/95cd1b246c2eacb18e9fa8da977a44ee7faf1313))
* **api,security:** harden CRD/admission contracts and guardrails ([#106](https://github.com/dc-tec/openbao-operator/issues/106)) ([40f49d8](https://github.com/dc-tec/openbao-operator/commit/40f49d890a757c3623f08142355fb5c1db3ad5e6))
* **api:** switch SecretReference to LocalObjectReference ([c3b8fef](https://github.com/dc-tec/openbao-operator/commit/c3b8fefd41e8f06b1b4456f66861974d06de4428))
* **auth:** harden OIDC discovery and add least-privilege RBAC + admission guardrails ([#86](https://github.com/dc-tec/openbao-operator/issues/86)) ([d128a5d](https://github.com/dc-tec/openbao-operator/commit/d128a5d653aa504bbaaadaf48dbd240fc8c7c8da))
* **auth:** harden operator OIDC bootstrap discovery ([#242](https://github.com/dc-tec/openbao-operator/issues/242)) ([c6fef5d](https://github.com/dc-tec/openbao-operator/commit/c6fef5d05860dab3de42f37cf45c9360c9723986))
* **auth:** retry kubernetes jwks discovery via api service ([#241](https://github.com/dc-tec/openbao-operator/issues/241)) ([37358f6](https://github.com/dc-tec/openbao-operator/commit/37358f65677819cd8d9ac52cd9775ebe718f23ea))
* **backup:** align retention behavior across providers and refactor backup/restore flow ([#105](https://github.com/dc-tec/openbao-operator/issues/105)) ([2e1fa9d](https://github.com/dc-tec/openbao-operator/commit/2e1fa9d941f818512155e34d6e7c8a9c6a620689))
* **backup:** make sure backup jobs are idempotent ([#47](https://github.com/dc-tec/openbao-operator/issues/47)) ([8e2ec6f](https://github.com/dc-tec/openbao-operator/commit/8e2ec6f058928a169718908b3e7fa38150ffcf80))
* **backup:** manual / scheduled backups ([f68172e](https://github.com/dc-tec/openbao-operator/commit/f68172e4800ce383d8a5b40e910465f6ad1ce86c))
* **backup:** remove unused function ([556161f](https://github.com/dc-tec/openbao-operator/commit/556161f542a71570fb94660a4d986a51df660a84))
* **backup:** upgrade paths ([e2bb9b5](https://github.com/dc-tec/openbao-operator/commit/e2bb9b5ceded236632ce89eee43a001efc0dca70))
* **bluegreen:** harden deterministic upgrade flow, tests, and docs ([#104](https://github.com/dc-tec/openbao-operator/issues/104)) ([bb64c2e](https://github.com/dc-tec/openbao-operator/commit/bb64c2ed593962f94c004971ec0986270a5270e0))
* **build:** stabilize byte reproducibility gates for checksums and sbom outputs ([#180](https://github.com/dc-tec/openbao-operator/issues/180)) ([7547ea4](https://github.com/dc-tec/openbao-operator/commit/7547ea48876ddda4788a4d004da31f5f4ea7b985))
* **chart:** sync helm chart ([9c22829](https://github.com/dc-tec/openbao-operator/commit/9c228297ace116396f351290620eb44991739d57))
* **chart:** sync helm chart ([#7](https://github.com/dc-tec/openbao-operator/issues/7)) ([507c364](https://github.com/dc-tec/openbao-operator/commit/507c36400b8f83b75e614df3fd34fcddd0e12283))
* **ci:** allow PR label sync to write labels ([#307](https://github.com/dc-tec/openbao-operator/issues/307)) ([51591d8](https://github.com/dc-tec/openbao-operator/commit/51591d8a212019134cb290d3c876385b08745e01))
* **ci:** always run perf weekly issue job after failed schedule check ([3d0eb18](https://github.com/dc-tec/openbao-operator/commit/3d0eb189ccda2545def4e3635dd5aabb8a24c599))
* **ci:** create kind cluster in release e2e gate ([#135](https://github.com/dc-tec/openbao-operator/issues/135)) ([838fe67](https://github.com/dc-tec/openbao-operator/commit/838fe6744cdde4346fe000c092c8059700de0664))
* **ci:** handle kind load failures for multi-arch OpenBao images ([#125](https://github.com/dc-tec/openbao-operator/issues/125)) ([05038ba](https://github.com/dc-tec/openbao-operator/commit/05038baaf0a706ee4c4c1c1d944f93a84c4768f0))
* **ci:** harden mainline publish workflows ([#224](https://github.com/dc-tec/openbao-operator/issues/224)) ([3bebc04](https://github.com/dc-tec/openbao-operator/commit/3bebc04970d43c77ba7fc7bcfac5cc7c63a18937))
* **ci:** replace dangerous PR labeling workflow ([#304](https://github.com/dc-tec/openbao-operator/issues/304)) ([b3740f8](https://github.com/dc-tec/openbao-operator/commit/b3740f89f65379b734ac70e8db5cd5982e479939))
* **ci:** restore security and bot PR pipeline stability ([#129](https://github.com/dc-tec/openbao-operator/issues/129)) ([ae8d297](https://github.com/dc-tec/openbao-operator/commit/ae8d297eae7ed5673d919673167ac4bdea002e1c))
* **ci:** stabilize nightly e2e image refs and matrix check naming ([#121](https://github.com/dc-tec/openbao-operator/issues/121)) ([c69993d](https://github.com/dc-tec/openbao-operator/commit/c69993d4eace0c5104aaf1659f390a25fadb4b69))
* **ci:** stabilize release/build reproducibility and align CI documentation ([#179](https://github.com/dc-tec/openbao-operator/issues/179)) ([4378cfe](https://github.com/dc-tec/openbao-operator/commit/4378cfe9cf33c35b87ea429290608a2d6a3f0c18))
* **ci:** unblock draft release lookup and run reproducibility post-release ([#185](https://github.com/dc-tec/openbao-operator/issues/185)) ([4fa1089](https://github.com/dc-tec/openbao-operator/commit/4fa10896da12c125cf7873567fd0e49876299517))
* **controller:** infer BlueImage from running pods to prevent premature upgrades ([#95](https://github.com/dc-tec/openbao-operator/issues/95)) ([dfdc11e](https://github.com/dc-tec/openbao-operator/commit/dfdc11efe964fa427b69cfebf0b22bac0fa98d3e))
* **controller:** persist initialized status ([c2ebbd1](https://github.com/dc-tec/openbao-operator/commit/c2ebbd1b6701982fbf5881d71c5a073d35f9854d))
* **controller:** Prevent data loss by orphaning secrets when DeletionPolicy is Retain ([#11](https://github.com/dc-tec/openbao-operator/issues/11)) ([0899cfa](https://github.com/dc-tec/openbao-operator/commit/0899cfa44d53deea6aaf65343d44b61c6a488168))
* **controller:** prevent OpenBaoCluster resourceVersion churn ([#49](https://github.com/dc-tec/openbao-operator/issues/49)) ([c0e4fe8](https://github.com/dc-tec/openbao-operator/commit/c0e4fe88c628cec4cab6ed6cd1bc053378f27d1e))
* **controller:** recheck admission dependencies at runtime ([#262](https://github.com/dc-tec/openbao-operator/issues/262)) ([8203a59](https://github.com/dc-tec/openbao-operator/commit/8203a59048f54c1b89a5862235b602cc9b0fb376))
* **controller:** refresh cluster status on standard cadence ([#257](https://github.com/dc-tec/openbao-operator/issues/257)) ([5fd50f3](https://github.com/dc-tec/openbao-operator/commit/5fd50f371870d3012c485e93b2839a7394cd272a))
* **controller:** remove force ownership of status ([#70](https://github.com/dc-tec/openbao-operator/issues/70)) ([e59e5da](https://github.com/dc-tec/openbao-operator/commit/e59e5da6d22ea82dde7c8c272447e4744991b51e))
* **controller:** strengthen status updates with patching ([6c54a5e](https://github.com/dc-tec/openbao-operator/commit/6c54a5e4505c8ff17756d8cc477b75641eaebedc))
* **controller:** timeout for image verification ([cbcd9cf](https://github.com/dc-tec/openbao-operator/commit/cbcd9cf753ee6b33d0167d8195cfff69a13e966c))
* **core:** add temporary transient error ([e0aeb21](https://github.com/dc-tec/openbao-operator/commit/e0aeb2146e9e713226c8aeecab06508d69295b2d))
* **core:** check token existence ([f4669f5](https://github.com/dc-tec/openbao-operator/commit/f4669f5b2cc2fe844e02e1282e8e3e8e12d5763a))
* **core:** decouple openbao client logic ([d3a0acc](https://github.com/dc-tec/openbao-operator/commit/d3a0acc6323aa7cda1e20b47189576f3a094bb0b))
* **core:** harden controller determinism and idempotency  ([#107](https://github.com/dc-tec/openbao-operator/issues/107)) ([e573bf9](https://github.com/dc-tec/openbao-operator/commit/e573bf96702c4fca761c34456b9898f5d7d63e90))
* **core:** improve container status checking ([e357dcc](https://github.com/dc-tec/openbao-operator/commit/e357dcc0fd385adb4b6a400eaca3cd84ef52bcc4))
* **core:** rbac and admission hardening ([477be64](https://github.com/dc-tec/openbao-operator/commit/477be6472cd6d45324b2ec879a70d50bd10fcf2f))
* **deps:** resolve security vulnerabilities in go-tuf/v2 and rekor dependencies ([#74](https://github.com/dc-tec/openbao-operator/issues/74)) ([ecbfba8](https://github.com/dc-tec/openbao-operator/commit/ecbfba80715689bf0eb1689ec370befbfad6cd83))
* **e2e:** sentinel drift detection robustness ([648f3df](https://github.com/dc-tec/openbao-operator/commit/648f3df3b08e71633f172993a22ecaf0559acfdb))
* **e2e:** unused param ([b7a9c02](https://github.com/dc-tec/openbao-operator/commit/b7a9c0294172e5d52c048e117a874239ffb6d10a))
* **helm:** allow global values in chart schema ([#378](https://github.com/dc-tec/openbao-operator/issues/378)) ([5dad02e](https://github.com/dc-tec/openbao-operator/commit/5dad02ebc4253ddb366f636e3aea60ffce5f4ffa))
* **helm:** Helm provisioner admission identity ([#387](https://github.com/dc-tec/openbao-operator/issues/387)) ([f781c70](https://github.com/dc-tec/openbao-operator/commit/f781c70b885973b0d682cc102607d3e0b41f36dd))
* **images:** fail-fast on missing OPERATOR_VERSION environment variable ([#25](https://github.com/dc-tec/openbao-operator/issues/25)) ([1a42097](https://github.com/dc-tec/openbao-operator/commit/1a42097c8fd80bfe773682865c1119b29ca77d02))
* Implement versioned default images for backup, upgrade, and init container ([#14](https://github.com/dc-tec/openbao-operator/issues/14)) ([1b34f78](https://github.com/dc-tec/openbao-operator/commit/1b34f785009750a2667293d31334260fee04716d))
* **infra:** add IPv6/dual-stack support for listener binding and development egress rules ([#56](https://github.com/dc-tec/openbao-operator/issues/56)) ([7bfdb41](https://github.com/dc-tec/openbao-operator/commit/7bfdb41840bed338cbfcede82be3aea6642a7a53))
* **infra:** delete scaled-down raft PVCs ([#341](https://github.com/dc-tec/openbao-operator/issues/341)) ([f406e90](https://github.com/dc-tec/openbao-operator/commit/f406e9029d94c8e7984d77b66cf02b8a97f3c339))
* **infra:** exclude job pods from pdb ([#9](https://github.com/dc-tec/openbao-operator/issues/9)) ([825a191](https://github.com/dc-tec/openbao-operator/commit/825a1916d68a6a0bb09c4f46c1251cf2af9cd159))
* **infra:** fail closed on hostile OIDC bootstrap discovery ([#263](https://github.com/dc-tec/openbao-operator/issues/263)) ([2dbd9be](https://github.com/dc-tec/openbao-operator/commit/2dbd9be4a01395d071af79876ef9cc9989cf606c))
* **infra:** improve initialization robustness by treating transient Secret/RBAC errors as retriable and hardening root-token creation ([#55](https://github.com/dc-tec/openbao-operator/issues/55)) ([f760ac5](https://github.com/dc-tec/openbao-operator/commit/f760ac5c17bd99f747e8c3dc637bdcee1b4cb511))
* **infra:** resolve BackendTLSPolicy mismatch and cleanup stale services after Blue/Green upgrade ([#10](https://github.com/dc-tec/openbao-operator/issues/10)) ([7052a54](https://github.com/dc-tec/openbao-operator/commit/7052a54145a4d9ac1a1d9ed3b7fdb1cc8de994a2))
* **infra:** stop apiserver endpoint autodetection; use service VIP allow-list with optional endpoint IPs ([#54](https://github.com/dc-tec/openbao-operator/issues/54)) ([d73179a](https://github.com/dc-tec/openbao-operator/commit/d73179a434428bb787684791d1de88dc778f138f))
* **init:** retrty writing root token to secret to handle transient cr… ([#84](https://github.com/dc-tec/openbao-operator/issues/84)) ([e100176](https://github.com/dc-tec/openbao-operator/commit/e1001769b05fbccae2c861b586dd3eac3eaefd8c))
* **kube:** add job check ([a7439a9](https://github.com/dc-tec/openbao-operator/commit/a7439a9fe060a4710deda76bea6b7bfafde18020))
* **manifests:** make JWT auth bootstrap a opt-in feature ([ded02a3](https://github.com/dc-tec/openbao-operator/commit/ded02a3173c672e7cbc03f5e993635e1cb345663))
* **manifests:** secure defaults and profiles ([6617383](https://github.com/dc-tec/openbao-operator/commit/66173839968834008119e07cf29cc99188ef8121))
* **multitenancy:** gate cluster reconcile on tenant onboarding ([#359](https://github.com/dc-tec/openbao-operator/issues/359)) ([cfd850f](https://github.com/dc-tec/openbao-operator/commit/cfd850fcf819c4d1562644cc9495143cfee69b27))
* **network:** Require source-scoped managed Ingress access ([#389](https://github.com/dc-tec/openbao-operator/issues/389)) ([a3cec85](https://github.com/dc-tec/openbao-operator/commit/a3cec85a56230560be8196ac02666ad38b7e136d))
* **nightly:** harden init token persistence and e2e autopilot reliability ([#117](https://github.com/dc-tec/openbao-operator/issues/117)) ([f85886f](https://github.com/dc-tec/openbao-operator/commit/f85886fc92b5df3eff30b5075659b41279e8717d))
* **openbao:** handle 403 forbidden gracefully ([#94](https://github.com/dc-tec/openbao-operator/issues/94)) ([4243f67](https://github.com/dc-tec/openbao-operator/commit/4243f67d68e69d8406b5e0702c806a4f876dd774))
* **openbao:** stage safe raft scale-downs ([#339](https://github.com/dc-tec/openbao-operator/issues/339)) ([4da1ec7](https://github.com/dc-tec/openbao-operator/commit/4da1ec74f8e4e45e710a0fae51f86bbf44c257c8))
* **probe:** stabilize openbao workload probes ([#371](https://github.com/dc-tec/openbao-operator/issues/371)) ([260547b](https://github.com/dc-tec/openbao-operator/commit/260547b71d3e12e2ec97ae500f9ed63ab1619804))
* **provisioner:** reduce release reconciliation log noise ([#370](https://github.com/dc-tec/openbao-operator/issues/370)) ([b2f2bca](https://github.com/dc-tec/openbao-operator/commit/b2f2bcaf18dfef15348aa02b9f3de224c02e38ab))
* **release-0.2:** backport 0.2.1 fixes ([069d9b4](https://github.com/dc-tec/openbao-operator/commit/069d9b454e95dda6c00788cc9878590a30e1146a))
* **release:** grant tag workflow comment permissions ([#295](https://github.com/dc-tec/openbao-operator/issues/295)) ([61ec413](https://github.com/dc-tec/openbao-operator/commit/61ec413d7b640e446d135e67e98bbc17c85badec))
* **release:** remove unsupported tag app scope ([#296](https://github.com/dc-tec/openbao-operator/issues/296)) ([e794a76](https://github.com/dc-tec/openbao-operator/commit/e794a7629f3ad31083834a7d5b0f63d64cc4b93e))
* **release:** sign release tags and trim release gates ([#298](https://github.com/dc-tec/openbao-operator/issues/298)) ([33a687b](https://github.com/dc-tec/openbao-operator/commit/33a687b9b93537bffd944791d7f02fc7d48fe855))
* **rolling:** handle retry status conflicts during upgrade resume ([#192](https://github.com/dc-tec/openbao-operator/issues/192)) ([c6957f2](https://github.com/dc-tec/openbao-operator/commit/c6957f280e1264b7912d0304d5937d6227b8a5f2))
* **security;e2e:** verify signed hardened/acme flows in CI/nightly and support digest-safe keyless defaults ([#116](https://github.com/dc-tec/openbao-operator/issues/116)) ([3b966fe](https://github.com/dc-tec/openbao-operator/commit/3b966fe25097fbb4e490682f93bc8671463741f2))
* **security:** fail closed for configured trusted roots ([#393](https://github.com/dc-tec/openbao-operator/issues/393)) ([04cbd64](https://github.com/dc-tec/openbao-operator/commit/04cbd64cf0356f111f0e3c0450b859008e6c5b69))
* **security:** harden managed image digests and gateway validation reads ([#243](https://github.com/dc-tec/openbao-operator/issues/243)) ([62a44d0](https://github.com/dc-tec/openbao-operator/commit/62a44d006fc27019e2f5cc1fa58ddb216e088503))
* **security:** implement image verification LRU cache; docker auth handeling ([#18](https://github.com/dc-tec/openbao-operator/issues/18)) ([a4b7203](https://github.com/dc-tec/openbao-operator/commit/a4b720313ec7fa40a7b0123de4bbbbe090441c0e))
* **security:** performance issue image verification by reording cache lookups ([#12](https://github.com/dc-tec/openbao-operator/issues/12)) ([a5ca5eb](https://github.com/dc-tec/openbao-operator/commit/a5ca5eb1268d9afe98d8bcc0ce6c3dda0efde20c))
* **security:** remove resolved govulncheck ignores ([#249](https://github.com/dc-tec/openbao-operator/issues/249)) ([58be543](https://github.com/dc-tec/openbao-operator/commit/58be543c57c0b47b977271d1e51eb0baa49853f9))
* **security:** validate UMASK bounds in bao-wrapper ([#195](https://github.com/dc-tec/openbao-operator/issues/195)) ([08b5f8a](https://github.com/dc-tec/openbao-operator/commit/08b5f8a6a92325d176ba40e3c79a4106570ab029))
* **security:** wrap bundle fallback verification error ([#200](https://github.com/dc-tec/openbao-operator/issues/200)) ([827899e](https://github.com/dc-tec/openbao-operator/commit/827899ea077c149e93ccf7aaf3c9d333a45b37c5))
* **sentinel:** prevent noisy neighbors and thundering herd behavior ([57eb7bd](https://github.com/dc-tec/openbao-operator/commit/57eb7bdfd9b714e2d64c0954d5a36c260dde7efa))
* **sentinel:** rely on uuids instead of timestamps as sentinel triggerid ([#6](https://github.com/dc-tec/openbao-operator/issues/6)) ([f88b697](https://github.com/dc-tec/openbao-operator/commit/f88b697f6dc13f19cf9a00d2764a4ed0be58868d))
* **status:** make lifecycle status guidance more actionable ([#227](https://github.com/dc-tec/openbao-operator/issues/227)) ([6bf9147](https://github.com/dc-tec/openbao-operator/commit/6bf9147aa42231f0f2494c00f6c9d77924a7e292))
* **status:** mark unsafe admission mode not production-ready ([#391](https://github.com/dc-tec/openbao-operator/issues/391)) ([98022a3](https://github.com/dc-tec/openbao-operator/commit/98022a3925742e011dbb8ce1fb55c2c79c5a1496))
* **storage:** enforce storage class immutability consistently ([#215](https://github.com/dc-tec/openbao-operator/issues/215)) ([c0a551f](https://github.com/dc-tec/openbao-operator/commit/c0a551fd8e5e0c653d151de5b17990573767c333))
* **upgrade:** add metrics for upgrade ([936d71e](https://github.com/dc-tec/openbao-operator/commit/936d71edca1f111a40c8a04bd32910459c24fc93))
* **upgrade:** clear rolling retry failure state with merge status patch ([#205](https://github.com/dc-tec/openbao-operator/issues/205)) ([f4b47f9](https://github.com/dc-tec/openbao-operator/commit/f4b47f9403fdd1ea954dd7af902d194f7889b055))
* **upgrade:** complete SSA ownership migration ([#345](https://github.com/dc-tec/openbao-operator/issues/345)) ([eafa931](https://github.com/dc-tec/openbao-operator/commit/eafa9317acf33155cc7863924b5cb4a8725f97bc))
* **upgrade:** harden bluegreen and rolling recovery flakes ([#374](https://github.com/dc-tec/openbao-operator/issues/374)) ([62cf706](https://github.com/dc-tec/openbao-operator/commit/62cf706df50b8ff462e5893166fc61b83749b298))
* **upgrade:** harden OpenBaoCluster upgrade validation, recovery, and documentation ([#225](https://github.com/dc-tec/openbao-operator/issues/225)) ([a170c0a](https://github.com/dc-tec/openbao-operator/commit/a170c0acb3c835016f32483169d3c61e07ab26b3))
* **upgrade:** improve upgrade manager stability ([#13](https://github.com/dc-tec/openbao-operator/issues/13)) ([c6a1b34](https://github.com/dc-tec/openbao-operator/commit/c6a1b34a515e7ed4201d61cd2b564ba2b0a9b5bf))
* **upgrade:** make rolling upgrades deterministic and harden rolling upgrade coverage ([#103](https://github.com/dc-tec/openbao-operator/issues/103)) ([5f3edfd](https://github.com/dc-tec/openbao-operator/commit/5f3edfd3d1b111b3b07a8818aa743f523ab8d810))
* **upgrade:** revert partition update to MergeFrom to fix StatefulSet validation ([#52](https://github.com/dc-tec/openbao-operator/issues/52)) ([504c319](https://github.com/dc-tec/openbao-operator/commit/504c31970030519ed602f16ebc3d7be5b339d32c))
* **upgrade:** set executor job resource requirements ([#392](https://github.com/dc-tec/openbao-operator/issues/392)) ([8efb8da](https://github.com/dc-tec/openbao-operator/commit/8efb8da900d378139e35bd32c54489bcc74bec15))
* **upgrade:** treat raft promote already-voter as no-op ([#382](https://github.com/dc-tec/openbao-operator/issues/382)) ([7d25753](https://github.com/dc-tec/openbao-operator/commit/7d25753b9c5c780e174e8adb5487f48c67128267))
* **upgrade:** use SSA for upgrade manager ([d0c289c](https://github.com/dc-tec/openbao-operator/commit/d0c289ce76686f7329e79cbbbfdc29b172446c74))
* **upgrade:** verify default helper images for hardened clusters ([#308](https://github.com/dc-tec/openbao-operator/issues/308)) ([8bfeabb](https://github.com/dc-tec/openbao-operator/commit/8bfeabb6b79a8d897617b0aac63d89be9530ef16))
* **validation:** block upgrade strategy switches ([#288](https://github.com/dc-tec/openbao-operator/issues/288)) ([b5f0af4](https://github.com/dc-tec/openbao-operator/commit/b5f0af4a7e5c7fbceb733a52e4bc3327171f93c6))
* **vap:** require self init requests when self initialization is enabled ([#82](https://github.com/dc-tec/openbao-operator/issues/82)) ([c572aaa](https://github.com/dc-tec/openbao-operator/commit/c572aaa392ecc8c8f6dccdee5203a964055a6106))
* **vap:** stuck Job deletions by allowing GC Job-finalizer updates in lock-managed-resource-mutations VAP ([#53](https://github.com/dc-tec/openbao-operator/issues/53)) ([0c56a87](https://github.com/dc-tec/openbao-operator/commit/0c56a8726c3a972566fc4a93b8a8d3d9bbd99ae7))


### Miscellaneous Chores

* **release:** release 0.1.0 ([#302](https://github.com/dc-tec/openbao-operator/issues/302)) ([ebcaf03](https://github.com/dc-tec/openbao-operator/commit/ebcaf03b7ca60a02d56e64135a45e6f1e20be424))
* **release:** release 0.1.0-rc.7 ([#299](https://github.com/dc-tec/openbao-operator/issues/299)) ([f1aa990](https://github.com/dc-tec/openbao-operator/commit/f1aa990e7ac08d4cf203d61ede7fd8b3448419bd))
* **release:** set release target to 0.1.0-rc.1 ([#133](https://github.com/dc-tec/openbao-operator/issues/133)) ([ad509ed](https://github.com/dc-tec/openbao-operator/commit/ad509edfa50936cc8b263fcae1d1233fa6b9f47b))
* **release:** set release target to 0.1.0-rc.2 ([#136](https://github.com/dc-tec/openbao-operator/issues/136)) ([624238d](https://github.com/dc-tec/openbao-operator/commit/624238df4f561709ce0390f3332c0737685d7a67))
* **release:** set release target to 0.1.0-rc.3 ([#176](https://github.com/dc-tec/openbao-operator/issues/176)) ([af6043e](https://github.com/dc-tec/openbao-operator/commit/af6043ee5c02d6440b9de9401ce8bb9c332831ba))
* **release:** set release target to 0.1.0-rc.4 ([#183](https://github.com/dc-tec/openbao-operator/issues/183)) ([b5402ea](https://github.com/dc-tec/openbao-operator/commit/b5402eaed71cf776dfa6b6a42b23c5030b38896c))
* **release:** set release target to 0.1.0-rc.5 ([#187](https://github.com/dc-tec/openbao-operator/issues/187)) ([39649ee](https://github.com/dc-tec/openbao-operator/commit/39649ee68ef28ed3c94cfebf2dc9de04f3ff2466))
* trigger release-please for 0.1.0-rc.6 ([#293](https://github.com/dc-tec/openbao-operator/issues/293)) ([9f8bfa1](https://github.com/dc-tec/openbao-operator/commit/9f8bfa193a8bb45d3327f99a6e365e49cab9879c))


### Code Refactoring

* **config:** openbaocluster config renderer ([a230262](https://github.com/dc-tec/openbao-operator/commit/a230262c4795566c21ad58a65b74364e7cdd36b6))
* **controller:** openbaocluster refactor; sentinel improvements ([9d0de98](https://github.com/dc-tec/openbao-operator/commit/9d0de984d9681d53f4c5569ff84443ae46e2bad5))
* **core:** remove Sentinel drift detection (VAP hardening) ([#39](https://github.com/dc-tec/openbao-operator/issues/39)) ([d289cf2](https://github.com/dc-tec/openbao-operator/commit/d289cf262213ab13ca3c9e3631df1d4845ee6fc7))
* **upgrade:** simplify blue/green cutover and split rolling strategy ([#37](https://github.com/dc-tec/openbao-operator/issues/37)) ([7453e23](https://github.com/dc-tec/openbao-operator/commit/7453e23880b1edbfa0c825d6982c29893d4ac08d))
* **upgrade:** upgrade manager; blue/green upgrades ([2ba56a4](https://github.com/dc-tec/openbao-operator/commit/2ba56a426caa12a79a069700b0b2a4ede44156e1))

## [0.2.1](https://github.com/dc-tec/openbao-operator/compare/0.2.0...0.2.1) (2026-05-18)


### Bug Fixes

* **release-0.2:** backport 0.2.1 fixes ([069d9b4](https://github.com/dc-tec/openbao-operator/commit/069d9b454e95dda6c00788cc9878590a30e1146a))

## [0.2.0](https://github.com/dc-tec/openbao-operator/compare/0.1.1...0.2.0) (2026-05-01)


### Features

* **admission:** authorize maintenance through RBAC ([#347](https://github.com/dc-tec/openbao-operator/issues/347)) ([b7c05a7](https://github.com/dc-tec/openbao-operator/commit/b7c05a770bcc97ea1931caf0a3c05919540c38ab))
* **api:** add runtime restart controls ([#348](https://github.com/dc-tec/openbao-operator/issues/348)) ([b1efd34](https://github.com/dc-tec/openbao-operator/commit/b1efd3442c2c5cd0a58c654b749103ab7cf5ac81))
* **readreplicas:** add steady-state read replica topology and status ([#361](https://github.com/dc-tec/openbao-operator/issues/361)) ([9a74c14](https://github.com/dc-tec/openbao-operator/commit/9a74c143e9061f42f5c7557af7a7e9b767252926))
* **readreplicas:** integrate read replicas with upgrade and restore workflows ([#362](https://github.com/dc-tec/openbao-operator/issues/362)) ([e8bf8b8](https://github.com/dc-tec/openbao-operator/commit/e8bf8b820c06ccab1fb81a9df25223dfbf4e0666))


### Bug Fixes

* **admission:** guard hardened security context overrides ([#390](https://github.com/dc-tec/openbao-operator/issues/390)) ([d0a6533](https://github.com/dc-tec/openbao-operator/commit/d0a6533a4c5dbb7b23e4c0c83abf6ee07a5b491e))
* **helm:** allow global values in chart schema ([#378](https://github.com/dc-tec/openbao-operator/issues/378)) ([5dad02e](https://github.com/dc-tec/openbao-operator/commit/5dad02ebc4253ddb366f636e3aea60ffce5f4ffa))
* **helm:** Helm provisioner admission identity ([#387](https://github.com/dc-tec/openbao-operator/issues/387)) ([f781c70](https://github.com/dc-tec/openbao-operator/commit/f781c70b885973b0d682cc102607d3e0b41f36dd))
* **infra:** delete scaled-down raft PVCs ([#341](https://github.com/dc-tec/openbao-operator/issues/341)) ([f406e90](https://github.com/dc-tec/openbao-operator/commit/f406e9029d94c8e7984d77b66cf02b8a97f3c339))
* **multitenancy:** gate cluster reconcile on tenant onboarding ([#359](https://github.com/dc-tec/openbao-operator/issues/359)) ([cfd850f](https://github.com/dc-tec/openbao-operator/commit/cfd850fcf819c4d1562644cc9495143cfee69b27))
* **network:** Require source-scoped managed Ingress access ([#389](https://github.com/dc-tec/openbao-operator/issues/389)) ([a3cec85](https://github.com/dc-tec/openbao-operator/commit/a3cec85a56230560be8196ac02666ad38b7e136d))
* **openbao:** stage safe raft scale-downs ([#339](https://github.com/dc-tec/openbao-operator/issues/339)) ([4da1ec7](https://github.com/dc-tec/openbao-operator/commit/4da1ec74f8e4e45e710a0fae51f86bbf44c257c8))
* **probe:** stabilize openbao workload probes ([#371](https://github.com/dc-tec/openbao-operator/issues/371)) ([260547b](https://github.com/dc-tec/openbao-operator/commit/260547b71d3e12e2ec97ae500f9ed63ab1619804))
* **provisioner:** reduce release reconciliation log noise ([#370](https://github.com/dc-tec/openbao-operator/issues/370)) ([b2f2bca](https://github.com/dc-tec/openbao-operator/commit/b2f2bcaf18dfef15348aa02b9f3de224c02e38ab))
* **security:** fail closed for configured trusted roots ([#393](https://github.com/dc-tec/openbao-operator/issues/393)) ([04cbd64](https://github.com/dc-tec/openbao-operator/commit/04cbd64cf0356f111f0e3c0450b859008e6c5b69))
* **status:** mark unsafe admission mode not production-ready ([#391](https://github.com/dc-tec/openbao-operator/issues/391)) ([98022a3](https://github.com/dc-tec/openbao-operator/commit/98022a3925742e011dbb8ce1fb55c2c79c5a1496))
* **upgrade:** complete SSA ownership migration ([#345](https://github.com/dc-tec/openbao-operator/issues/345)) ([eafa931](https://github.com/dc-tec/openbao-operator/commit/eafa9317acf33155cc7863924b5cb4a8725f97bc))
* **upgrade:** harden bluegreen and rolling recovery flakes ([#374](https://github.com/dc-tec/openbao-operator/issues/374)) ([62cf706](https://github.com/dc-tec/openbao-operator/commit/62cf706df50b8ff462e5893166fc61b83749b298))
* **upgrade:** set executor job resource requirements ([#392](https://github.com/dc-tec/openbao-operator/issues/392)) ([8efb8da](https://github.com/dc-tec/openbao-operator/commit/8efb8da900d378139e35bd32c54489bcc74bec15))
* **upgrade:** treat raft promote already-voter as no-op ([#382](https://github.com/dc-tec/openbao-operator/issues/382)) ([7d25753](https://github.com/dc-tec/openbao-operator/commit/7d25753b9c5c780e174e8adb5487f48c67128267))

## [0.1.1](https://github.com/dc-tec/openbao-operator/compare/0.1.0...0.1.1) (2026-03-31)


### Bug Fixes

* **ci:** allow PR label sync to write labels ([#307](https://github.com/dc-tec/openbao-operator/issues/307)) ([51591d8](https://github.com/dc-tec/openbao-operator/commit/51591d8a212019134cb290d3c876385b08745e01))
* **ci:** replace dangerous PR labeling workflow ([#304](https://github.com/dc-tec/openbao-operator/issues/304)) ([b3740f8](https://github.com/dc-tec/openbao-operator/commit/b3740f89f65379b734ac70e8db5cd5982e479939))
* **upgrade:** verify default helper images for hardened clusters ([#308](https://github.com/dc-tec/openbao-operator/issues/308)) ([8bfeabb](https://github.com/dc-tec/openbao-operator/commit/8bfeabb6b79a8d897617b0aac63d89be9530ef16))

## [0.1.0](https://github.com/dc-tec/openbao-operator/compare/0.1.0-rc.7...0.1.0) (2026-03-30)

The first stable pre-GA release of OpenBao Operator. This release rolls up the
`0.1.0-rc.1` through `0.1.0-rc.7` line into the initial `0.1.0` baseline.

### Highlights

* cluster lifecycle management for OpenBao on Kubernetes, including bootstrap,
  self-init, and day-2 operations
* rolling and blue/green upgrade workflows, with backup and restore support
* tenant onboarding for the default multi-tenant model, plus supported
  single-tenant installation paths
* hardened security posture with image verification, admission guardrails, RBAC
  hardening, and stronger lifecycle validation
* improved observability and operator-facing status, events, and operational
  guidance
* stable Helm chart packaging, signed release tags, signed artifacts, SBOMs,
  and published attestations

### Operational Notes

* The served API remains `openbao.org/v1alpha1`; pre-`1.0` minor releases may
  still carry breaking changes.
* See the `0.1.0-rc.1` through `0.1.0-rc.7` entries below for the detailed
  incremental history behind this release.
* See the project documentation for current compatibility, support policy, and
  known limitations.

## [0.1.0-rc.7](https://github.com/dc-tec/openbao-operator/compare/0.1.0-rc.6...0.1.0-rc.7) (2026-03-30)


### Bug Fixes

* **release:** grant tag workflow comment permissions ([#295](https://github.com/dc-tec/openbao-operator/issues/295)) ([61ec413](https://github.com/dc-tec/openbao-operator/commit/61ec413d7b640e446d135e67e98bbc17c85badec))
* **release:** remove unsupported tag app scope ([#296](https://github.com/dc-tec/openbao-operator/issues/296)) ([e794a76](https://github.com/dc-tec/openbao-operator/commit/e794a7629f3ad31083834a7d5b0f63d64cc4b93e))
* **release:** sign release tags and trim release gates ([#298](https://github.com/dc-tec/openbao-operator/issues/298)) ([33a687b](https://github.com/dc-tec/openbao-operator/commit/33a687b9b93537bffd944791d7f02fc7d48fe855))


### Miscellaneous Chores

* **release:** release 0.1.0-rc.7 ([#299](https://github.com/dc-tec/openbao-operator/issues/299)) ([f1aa990](https://github.com/dc-tec/openbao-operator/commit/f1aa990e7ac08d4cf203d61ede7fd8b3448419bd))

## [0.1.0-rc.6](https://github.com/dc-tec/openbao-operator/compare/0.1.0-rc.5...0.1.0-rc.6) (2026-03-30)


### Features

* **api:** add OpenBaoCluster observedGeneration and printer columns ([#286](https://github.com/dc-tec/openbao-operator/issues/286)) ([1c8f8ae](https://github.com/dc-tec/openbao-operator/commit/1c8f8aeb143fd90ca6452d2f72852c47b14ab5ea))
* **ast-grep:** add policy-driven architecture guardrails with CI enforcement ([#201](https://github.com/dc-tec/openbao-operator/issues/201)) ([1faee9a](https://github.com/dc-tec/openbao-operator/commit/1faee9a6b000d0e68770d7e2894e68d66f13f534))
* **core:** add consistent Kubernetes lifecycle events ([#226](https://github.com/dc-tec/openbao-operator/issues/226)) ([93687af](https://github.com/dc-tec/openbao-operator/commit/93687af087760053b01de76dc6a050e3f5c9e280))
* **core:** harden lifecycle contracts and supporting coverage ([#237](https://github.com/dc-tec/openbao-operator/issues/237)) ([44de947](https://github.com/dc-tec/openbao-operator/commit/44de94790ed765a8eb4036490858139b6a8561bd))
* **infra:** add default node and zone spreading for OpenBao StatefulSets ([#214](https://github.com/dc-tec/openbao-operator/issues/214)) ([1d7afc8](https://github.com/dc-tec/openbao-operator/commit/1d7afc8d55ddede8b24207274101a13d4352e98a))
* **infra:** add pod metadata hooks for workload identity ([#216](https://github.com/dc-tec/openbao-operator/issues/216)) ([9bd2546](https://github.com/dc-tec/openbao-operator/commit/9bd2546ccf5caf0024263073635a2a87ad6713c1))
* **operator:** add supported single-tenant custom identity install paths ([#239](https://github.com/dc-tec/openbao-operator/issues/239)) ([d41ff74](https://github.com/dc-tec/openbao-operator/commit/d41ff74b33133bd05bb2b2a7dadcaf4e4fe3305a))
* **security:** tighten operator security and authentication contracts ([#238](https://github.com/dc-tec/openbao-operator/issues/238)) ([7b14fb1](https://github.com/dc-tec/openbao-operator/commit/7b14fb1cc9046cd469451c3d1d8bb4cb0cbb0302))
* **upgrade:** unify manual upgrade requests on OpenBaoCluster ([#228](https://github.com/dc-tec/openbao-operator/issues/228)) ([b6f6848](https://github.com/dc-tec/openbao-operator/commit/b6f68487add3723932ff454f18d63f0c6688cac5))


### Bug Fixes

* **admission:** allow hardened image verification defaults ([#240](https://github.com/dc-tec/openbao-operator/issues/240)) ([817f144](https://github.com/dc-tec/openbao-operator/commit/817f144a066b21bf05040dd03d35e45ea37b8eb3))
* **auth:** harden operator OIDC bootstrap discovery ([#242](https://github.com/dc-tec/openbao-operator/issues/242)) ([c6fef5d](https://github.com/dc-tec/openbao-operator/commit/c6fef5d05860dab3de42f37cf45c9360c9723986))
* **auth:** retry kubernetes jwks discovery via api service ([#241](https://github.com/dc-tec/openbao-operator/issues/241)) ([37358f6](https://github.com/dc-tec/openbao-operator/commit/37358f65677819cd8d9ac52cd9775ebe718f23ea))
* **ci:** always run perf weekly issue job after failed schedule check ([3d0eb18](https://github.com/dc-tec/openbao-operator/commit/3d0eb189ccda2545def4e3635dd5aabb8a24c599))
* **ci:** harden mainline publish workflows ([#224](https://github.com/dc-tec/openbao-operator/issues/224)) ([3bebc04](https://github.com/dc-tec/openbao-operator/commit/3bebc04970d43c77ba7fc7bcfac5cc7c63a18937))
* **controller:** recheck admission dependencies at runtime ([#262](https://github.com/dc-tec/openbao-operator/issues/262)) ([8203a59](https://github.com/dc-tec/openbao-operator/commit/8203a59048f54c1b89a5862235b602cc9b0fb376))
* **controller:** refresh cluster status on standard cadence ([#257](https://github.com/dc-tec/openbao-operator/issues/257)) ([5fd50f3](https://github.com/dc-tec/openbao-operator/commit/5fd50f371870d3012c485e93b2839a7394cd272a))
* **infra:** fail closed on hostile OIDC bootstrap discovery ([#263](https://github.com/dc-tec/openbao-operator/issues/263)) ([2dbd9be](https://github.com/dc-tec/openbao-operator/commit/2dbd9be4a01395d071af79876ef9cc9989cf606c))
* **rolling:** handle retry status conflicts during upgrade resume ([#192](https://github.com/dc-tec/openbao-operator/issues/192)) ([c6957f2](https://github.com/dc-tec/openbao-operator/commit/c6957f280e1264b7912d0304d5937d6227b8a5f2))
* **security:** harden managed image digests and gateway validation reads ([#243](https://github.com/dc-tec/openbao-operator/issues/243)) ([62a44d0](https://github.com/dc-tec/openbao-operator/commit/62a44d006fc27019e2f5cc1fa58ddb216e088503))
* **security:** remove resolved govulncheck ignores ([#249](https://github.com/dc-tec/openbao-operator/issues/249)) ([58be543](https://github.com/dc-tec/openbao-operator/commit/58be543c57c0b47b977271d1e51eb0baa49853f9))
* **security:** validate UMASK bounds in bao-wrapper ([#195](https://github.com/dc-tec/openbao-operator/issues/195)) ([08b5f8a](https://github.com/dc-tec/openbao-operator/commit/08b5f8a6a92325d176ba40e3c79a4106570ab029))
* **security:** wrap bundle fallback verification error ([#200](https://github.com/dc-tec/openbao-operator/issues/200)) ([827899e](https://github.com/dc-tec/openbao-operator/commit/827899ea077c149e93ccf7aaf3c9d333a45b37c5))
* **status:** make lifecycle status guidance more actionable ([#227](https://github.com/dc-tec/openbao-operator/issues/227)) ([6bf9147](https://github.com/dc-tec/openbao-operator/commit/6bf9147aa42231f0f2494c00f6c9d77924a7e292))
* **storage:** enforce storage class immutability consistently ([#215](https://github.com/dc-tec/openbao-operator/issues/215)) ([c0a551f](https://github.com/dc-tec/openbao-operator/commit/c0a551fd8e5e0c653d151de5b17990573767c333))
* **upgrade:** clear rolling retry failure state with merge status patch ([#205](https://github.com/dc-tec/openbao-operator/issues/205)) ([f4b47f9](https://github.com/dc-tec/openbao-operator/commit/f4b47f9403fdd1ea954dd7af902d194f7889b055))
* **upgrade:** harden OpenBaoCluster upgrade validation, recovery, and documentation ([#225](https://github.com/dc-tec/openbao-operator/issues/225)) ([a170c0a](https://github.com/dc-tec/openbao-operator/commit/a170c0acb3c835016f32483169d3c61e07ab26b3))
* **validation:** block upgrade strategy switches ([#288](https://github.com/dc-tec/openbao-operator/issues/288)) ([b5f0af4](https://github.com/dc-tec/openbao-operator/commit/b5f0af4a7e5c7fbceb733a52e4bc3327171f93c6))


### Miscellaneous Chores

* trigger release-please for 0.1.0-rc.6 ([#293](https://github.com/dc-tec/openbao-operator/issues/293)) ([9f8bfa1](https://github.com/dc-tec/openbao-operator/commit/9f8bfa193a8bb45d3327f99a6e365e49cab9879c))

## [0.1.0-rc.5](https://github.com/dc-tec/openbao-operator/compare/0.1.0-rc.4...0.1.0-rc.5) (2026-03-01)


### Bug Fixes

* **ci:** unblock draft release lookup and run reproducibility post-release ([#185](https://github.com/dc-tec/openbao-operator/issues/185)) ([4fa1089](https://github.com/dc-tec/openbao-operator/commit/4fa10896da12c125cf7873567fd0e49876299517))


### Miscellaneous Chores

* **release:** set release target to 0.1.0-rc.5 ([#187](https://github.com/dc-tec/openbao-operator/issues/187)) ([39649ee](https://github.com/dc-tec/openbao-operator/commit/39649ee68ef28ed3c94cfebf2dc9de04f3ff2466))

## [0.1.0-rc.4](https://github.com/dc-tec/openbao-operator/compare/0.1.0-rc.3...0.1.0-rc.4) (2026-03-01)


### Bug Fixes

* **build:** stabilize byte reproducibility gates for checksums and sbom outputs ([#180](https://github.com/dc-tec/openbao-operator/issues/180)) ([7547ea4](https://github.com/dc-tec/openbao-operator/commit/7547ea48876ddda4788a4d004da31f5f4ea7b985))
* **ci:** stabilize release/build reproducibility and align CI documentation ([#179](https://github.com/dc-tec/openbao-operator/issues/179)) ([4378cfe](https://github.com/dc-tec/openbao-operator/commit/4378cfe9cf33c35b87ea429290608a2d6a3f0c18))


### Miscellaneous Chores

* **release:** set release target to 0.1.0-rc.4 ([#183](https://github.com/dc-tec/openbao-operator/issues/183)) ([b5402ea](https://github.com/dc-tec/openbao-operator/commit/b5402eaed71cf776dfa6b6a42b23c5030b38896c))

## [0.1.0-rc.3](https://github.com/dc-tec/openbao-operator/compare/0.1.0-rc.2...0.1.0-rc.3) (2026-02-28)


### Miscellaneous Chores

* **release:** set release target to 0.1.0-rc.3 ([#176](https://github.com/dc-tec/openbao-operator/issues/176)) ([af6043e](https://github.com/dc-tec/openbao-operator/commit/af6043ee5c02d6440b9de9401ce8bb9c332831ba))

## [0.1.0-rc.2](https://github.com/dc-tec/openbao-operator/compare/0.1.0-rc.1...0.1.0-rc.2) (2026-02-27)


### Bug Fixes

* **ci:** create kind cluster in release e2e gate ([#135](https://github.com/dc-tec/openbao-operator/issues/135)) ([838fe67](https://github.com/dc-tec/openbao-operator/commit/838fe6744cdde4346fe000c092c8059700de0664))


### Miscellaneous Chores

* **release:** set release target to 0.1.0-rc.2 ([#136](https://github.com/dc-tec/openbao-operator/issues/136)) ([624238d](https://github.com/dc-tec/openbao-operator/commit/624238df4f561709ce0390f3332c0737685d7a67))

## 0.1.0-rc.1 (2026-02-26)


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
* **core:** add perf baseline harness and gates ([#118](https://github.com/dc-tec/openbao-operator/issues/118)) ([bf91ce2](https://github.com/dc-tec/openbao-operator/commit/bf91ce24ec1de79cb96b1d1a1370938b62195dd7))
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
* **observability:** add metrics, dashboards, e2e assertions; upgrade stability ([#101](https://github.com/dc-tec/openbao-operator/issues/101)) ([d4ce07d](https://github.com/dc-tec/openbao-operator/commit/d4ce07dc4d895381066ca86962fc5758f66dfd33))
* **perf:** refresh kind performance baseline ([#120](https://github.com/dc-tec/openbao-operator/issues/120)) ([69e5366](https://github.com/dc-tec/openbao-operator/commit/69e5366651ac500925336358fb013c0e9650e4f2))
* **policy:** enforce Hardened profile requires replicas &gt;= 3 via VAP ([#23](https://github.com/dc-tec/openbao-operator/issues/23)) ([c15ab9f](https://github.com/dc-tec/openbao-operator/commit/c15ab9fd1421b613e138861a962f51cd76b721b3))
* **provisioner:** configurable tenant resource quotas ([#50](https://github.com/dc-tec/openbao-operator/issues/50)) ([4c6fc29](https://github.com/dc-tec/openbao-operator/commit/4c6fc2915cb821547129a6c9b8e1ed73e42fd500))
* **restore:** add RBAC for restore jobs and validate authentication ([#16](https://github.com/dc-tec/openbao-operator/issues/16)) ([e7772a1](https://github.com/dc-tec/openbao-operator/commit/e7772a146482c9626c545bddff185b9a2f687c1b))
* **security:** Add admission-time protections for SSRF, TLS secrets, and tenant self-service ([#51](https://github.com/dc-tec/openbao-operator/issues/51)) ([ae2f86c](https://github.com/dc-tec/openbao-operator/commit/ae2f86c851b1369676cee536b37dd934c8ef0d0a))
* **security:** add operatorimageVerification field to CRD to allow separate verification of both OpenBao and Operator images ([#8](https://github.com/dc-tec/openbao-operator/issues/8)) ([4c1b8cc](https://github.com/dc-tec/openbao-operator/commit/4c1b8cccd1d2c47618c29efa3d08c54535da421c))
* **security:** cosign keyless image verification ([0c60a60](https://github.com/dc-tec/openbao-operator/commit/0c60a60a530904695241707aba96cae7edf8390f))
* **security:** expand control-plane audit coverage for startup, operations, and RBAC mutations ([#109](https://github.com/dc-tec/openbao-operator/issues/109)) ([b32dc97](https://github.com/dc-tec/openbao-operator/commit/b32dc97175999aadb84cecf867395a7cca2a6f85))
* **security:** harden image verification and align edge/nightly signed manifest streams ([#112](https://github.com/dc-tec/openbao-operator/issues/112)) ([b755ca3](https://github.com/dc-tec/openbao-operator/commit/b755ca333c4e598cf5904b9e68817ac540393cc5))
* **security:** harden image verification defaults and sign edge/nightly images ([#111](https://github.com/dc-tec/openbao-operator/issues/111)) ([5ffed83](https://github.com/dc-tec/openbao-operator/commit/5ffed83ea179fe14fedba50320425d8e4ce0b30c))
* **security:** harden operator RBAC with ValidatingAdmissionPolicy guardrails ([#100](https://github.com/dc-tec/openbao-operator/issues/100)) ([643fd94](https://github.com/dc-tec/openbao-operator/commit/643fd94af7f0a128bf4f62fa073ffa70ec92af18))
* **test:** tlsroute; monitoring; backup/upgrade ([bc8497a](https://github.com/dc-tec/openbao-operator/commit/bc8497a112177ecf837ad3c935af4c90945caafb))
* **upgrade:** harden backup and restore flows ([cb542ab](https://github.com/dc-tec/openbao-operator/commit/cb542ab466e29ddbbf61460ebd9368891aa9e359))
* **upgrade:** improve upgrade manager stability by using SSA for status updates and make pre-upgrade backup job names deterministic ([#17](https://github.com/dc-tec/openbao-operator/issues/17)) ([78f6124](https://github.com/dc-tec/openbao-operator/commit/78f6124b7e3545149b86a167165fb081b7c810ac))
* **vap:** harden OpenBaoRestore VAP guardrails + allow default backup executor image ([#76](https://github.com/dc-tec/openbao-operator/issues/76)) ([93524c8](https://github.com/dc-tec/openbao-operator/commit/93524c8b91563bd5bee91caf2ef0d9360d0a2b04))


### Bug Fixes

* **admission:** add admission check ([50d3af0](https://github.com/dc-tec/openbao-operator/commit/50d3af0aa06773e5ea5ee98a1194cba7c9f98b1e))
* **admission:** implement security/rbac improvements ([95cd1b2](https://github.com/dc-tec/openbao-operator/commit/95cd1b246c2eacb18e9fa8da977a44ee7faf1313))
* **api,security:** harden CRD/admission contracts and guardrails ([#106](https://github.com/dc-tec/openbao-operator/issues/106)) ([40f49d8](https://github.com/dc-tec/openbao-operator/commit/40f49d890a757c3623f08142355fb5c1db3ad5e6))
* **api:** switch SecretReference to LocalObjectReference ([c3b8fef](https://github.com/dc-tec/openbao-operator/commit/c3b8fefd41e8f06b1b4456f66861974d06de4428))
* **auth:** harden OIDC discovery and add least-privilege RBAC + admission guardrails ([#86](https://github.com/dc-tec/openbao-operator/issues/86)) ([d128a5d](https://github.com/dc-tec/openbao-operator/commit/d128a5d653aa504bbaaadaf48dbd240fc8c7c8da))
* **backup:** align retention behavior across providers and refactor backup/restore flow ([#105](https://github.com/dc-tec/openbao-operator/issues/105)) ([2e1fa9d](https://github.com/dc-tec/openbao-operator/commit/2e1fa9d941f818512155e34d6e7c8a9c6a620689))
* **backup:** backup ([8bdc5fa](https://github.com/dc-tec/openbao-operator/commit/8bdc5fa0f00affcc6ca8c172f64bdb557a994b54))
* **backup:** make sure backup jobs are idempotent ([#47](https://github.com/dc-tec/openbao-operator/issues/47)) ([8e2ec6f](https://github.com/dc-tec/openbao-operator/commit/8e2ec6f058928a169718908b3e7fa38150ffcf80))
* **backup:** manual / scheduled backups ([f68172e](https://github.com/dc-tec/openbao-operator/commit/f68172e4800ce383d8a5b40e910465f6ad1ce86c))
* **backup:** pod security context hardening for init and backup containers ([cec43e6](https://github.com/dc-tec/openbao-operator/commit/cec43e6c7fa1e080ad4ec4d223bcb61d2106bbf2))
* **backup:** remove unused function ([556161f](https://github.com/dc-tec/openbao-operator/commit/556161f542a71570fb94660a4d986a51df660a84))
* **backup:** upgrade paths ([e2bb9b5](https://github.com/dc-tec/openbao-operator/commit/e2bb9b5ceded236632ce89eee43a001efc0dca70))
* **bluegreen:** harden deterministic upgrade flow, tests, and docs ([#104](https://github.com/dc-tec/openbao-operator/issues/104)) ([bb64c2e](https://github.com/dc-tec/openbao-operator/commit/bb64c2ed593962f94c004971ec0986270a5270e0))
* **chart:** sync helm chart ([9c22829](https://github.com/dc-tec/openbao-operator/commit/9c228297ace116396f351290620eb44991739d57))
* **chart:** sync helm chart ([#7](https://github.com/dc-tec/openbao-operator/issues/7)) ([507c364](https://github.com/dc-tec/openbao-operator/commit/507c36400b8f83b75e614df3fd34fcddd0e12283))
* **ci:** handle kind load failures for multi-arch OpenBao images ([#125](https://github.com/dc-tec/openbao-operator/issues/125)) ([05038ba](https://github.com/dc-tec/openbao-operator/commit/05038baaf0a706ee4c4c1c1d944f93a84c4768f0))
* **ci:** restore security and bot PR pipeline stability ([#129](https://github.com/dc-tec/openbao-operator/issues/129)) ([ae8d297](https://github.com/dc-tec/openbao-operator/commit/ae8d297eae7ed5673d919673167ac4bdea002e1c))
* **ci:** stabilize nightly e2e image refs and matrix check naming ([#121](https://github.com/dc-tec/openbao-operator/issues/121)) ([c69993d](https://github.com/dc-tec/openbao-operator/commit/c69993d4eace0c5104aaf1659f390a25fadb4b69))
* **controller:** infer BlueImage from running pods to prevent premature upgrades ([#95](https://github.com/dc-tec/openbao-operator/issues/95)) ([dfdc11e](https://github.com/dc-tec/openbao-operator/commit/dfdc11efe964fa427b69cfebf0b22bac0fa98d3e))
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
* **core:** harden controller determinism and idempotency  ([#107](https://github.com/dc-tec/openbao-operator/issues/107)) ([e573bf9](https://github.com/dc-tec/openbao-operator/commit/e573bf96702c4fca761c34456b9898f5d7d63e90))
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
* **nightly:** harden init token persistence and e2e autopilot reliability ([#117](https://github.com/dc-tec/openbao-operator/issues/117)) ([f85886f](https://github.com/dc-tec/openbao-operator/commit/f85886fc92b5df3eff30b5075659b41279e8717d))
* **openbao:** handle 403 forbidden gracefully ([#94](https://github.com/dc-tec/openbao-operator/issues/94)) ([4243f67](https://github.com/dc-tec/openbao-operator/commit/4243f67d68e69d8406b5e0702c806a4f876dd774))
* **security;e2e:** verify signed hardened/acme flows in CI/nightly and support digest-safe keyless defaults ([#116](https://github.com/dc-tec/openbao-operator/issues/116)) ([3b966fe](https://github.com/dc-tec/openbao-operator/commit/3b966fe25097fbb4e490682f93bc8671463741f2))
* **security:** implement image verification LRU cache; docker auth handeling ([#18](https://github.com/dc-tec/openbao-operator/issues/18)) ([a4b7203](https://github.com/dc-tec/openbao-operator/commit/a4b720313ec7fa40a7b0123de4bbbbe090441c0e))
* **security:** performance issue image verification by reording cache lookups ([#12](https://github.com/dc-tec/openbao-operator/issues/12)) ([a5ca5eb](https://github.com/dc-tec/openbao-operator/commit/a5ca5eb1268d9afe98d8bcc0ce6c3dda0efde20c))
* **sentinel:** prevent noisy neighbors and thundering herd behavior ([57eb7bd](https://github.com/dc-tec/openbao-operator/commit/57eb7bdfd9b714e2d64c0954d5a36c260dde7efa))
* **sentinel:** rely on uuids instead of timestamps as sentinel triggerid ([#6](https://github.com/dc-tec/openbao-operator/issues/6)) ([f88b697](https://github.com/dc-tec/openbao-operator/commit/f88b697f6dc13f19cf9a00d2764a4ed0be58868d))
* **upgrade:** add metrics for upgrade ([936d71e](https://github.com/dc-tec/openbao-operator/commit/936d71edca1f111a40c8a04bd32910459c24fc93))
* **upgrade:** improve upgrade manager stability ([#13](https://github.com/dc-tec/openbao-operator/issues/13)) ([c6a1b34](https://github.com/dc-tec/openbao-operator/commit/c6a1b34a515e7ed4201d61cd2b564ba2b0a9b5bf))
* **upgrade:** make rolling upgrades deterministic and harden rolling upgrade coverage ([#103](https://github.com/dc-tec/openbao-operator/issues/103)) ([5f3edfd](https://github.com/dc-tec/openbao-operator/commit/5f3edfd3d1b111b3b07a8818aa743f523ab8d810))
* **upgrade:** revert partition update to MergeFrom to fix StatefulSet validation ([#52](https://github.com/dc-tec/openbao-operator/issues/52)) ([504c319](https://github.com/dc-tec/openbao-operator/commit/504c31970030519ed602f16ebc3d7be5b339d32c))
* **upgrade:** use SSA for upgrade manager ([d0c289c](https://github.com/dc-tec/openbao-operator/commit/d0c289ce76686f7329e79cbbbfdc29b172446c74))
* **vap:** require self init requests when self initialization is enabled ([#82](https://github.com/dc-tec/openbao-operator/issues/82)) ([c572aaa](https://github.com/dc-tec/openbao-operator/commit/c572aaa392ecc8c8f6dccdee5203a964055a6106))
* **vap:** stuck Job deletions by allowing GC Job-finalizer updates in lock-managed-resource-mutations VAP ([#53](https://github.com/dc-tec/openbao-operator/issues/53)) ([0c56a87](https://github.com/dc-tec/openbao-operator/commit/0c56a8726c3a972566fc4a93b8a8d3d9bbd99ae7))


### Miscellaneous Chores

* **release:** set release target to 0.1.0-rc.1 ([#133](https://github.com/dc-tec/openbao-operator/issues/133)) ([ad509ed](https://github.com/dc-tec/openbao-operator/commit/ad509edfa50936cc8b263fcae1d1233fa6b9f47b))


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
