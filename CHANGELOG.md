# Changelog

This changelog is generated from git history using Conventional Commits.

## Unreleased

### core

#### Features

- introduce restore CRD ([`4d19b72`](https://github.com/dc-tec/openbao-operator/commit/4d19b72b5c74b337b61776f58f0d8f6ff711e8a9))
- blue/green upgrades ([`1a6783e`](https://github.com/dc-tec/openbao-operator/commit/1a6783eeb1cb45933d5cc146c81644fca26ccc11))
- introduce structured error types ([`0b17ae1`](https://github.com/dc-tec/openbao-operator/commit/0b17ae13e63ac49ac33d066df146ddb3190c6c40))

#### Fixes

- rbac and admission hardening ([`477be64`](https://github.com/dc-tec/openbao-operator/commit/477be6472cd6d45324b2ec879a70d50bd10fcf2f))
- improve container status checking ([`e357dcc`](https://github.com/dc-tec/openbao-operator/commit/e357dcc0fd385adb4b6a400eaca3cd84ef52bcc4))
- check token existence ([`f4669f5`](https://github.com/dc-tec/openbao-operator/commit/f4669f5b2cc2fe844e02e1282e8e3e8e12d5763a))
- add temporary transient error ([`e0aeb21`](https://github.com/dc-tec/openbao-operator/commit/e0aeb2146e9e713226c8aeecab06508d69295b2d))
- decouple openbao client logic ([`d3a0acc`](https://github.com/dc-tec/openbao-operator/commit/d3a0acc6323aa7cda1e20b47189576f3a094bb0b))
- centralize constants into internal/constants ([`058b0a3`](https://github.com/dc-tec/openbao-operator/commit/058b0a3b5abfc1733d49b6dbcbf66e5a4fbb3be4))

#### Refactors

- reconicle handeling ([`099fe57`](https://github.com/dc-tec/openbao-operator/commit/099fe5737b6a69afdc4d16b06042f742665af657))
- rbac and provisioner improvements ([`c986e67`](https://github.com/dc-tec/openbao-operator/commit/c986e677fd7c9d91ad93638848c2f7ff3c51a165))
- init manager ([`8adb4a8`](https://github.com/dc-tec/openbao-operator/commit/8adb4a8f641c494f6aedbe771ea06a7e09b7fc98))
- openbao api client improvements ([`fa77ac9`](https://github.com/dc-tec/openbao-operator/commit/fa77ac9a65e1e4b67455792cd6995c9e6e7cc5f8))
- move constants ([`dce489e`](https://github.com/dc-tec/openbao-operator/commit/dce489e83ccf4868f8918175bbec6974b6ce33df))
- api client ([`0a6e0dc`](https://github.com/dc-tec/openbao-operator/commit/0a6e0dc4ade53d9e8dd37a44b3848781884400eb))
- extract constants ([`11a2979`](https://github.com/dc-tec/openbao-operator/commit/11a29795ecfaaf3ca3f716cf8980d2475a71f242))

#### Tests

- add unit tests ([`92ed1da`](https://github.com/dc-tec/openbao-operator/commit/92ed1dace6254b7a38e83545f9091c2a6997d00b))

#### Chores

- move constants ([`a6f25a1`](https://github.com/dc-tec/openbao-operator/commit/a6f25a1d79f173fa4819dcac08f76c9557736ca0))

### manifests

#### Features

- wire-in image verification for all components ([`d94d1f9`](https://github.com/dc-tec/openbao-operator/commit/d94d1f9d14c81bd994124fc964e69787045fb646))
- install manifest ([`ffc63c6`](https://github.com/dc-tec/openbao-operator/commit/ffc63c669bd13c930c8e8f11ce465298e4ab4c0d))
- self-service tenant onboarding ([`2a8d4d0`](https://github.com/dc-tec/openbao-operator/commit/2a8d4d03bfdd53b86af93dbf4b6b4be9c9fcc9a7))
- optional sentinel deployment for quicker reconcile ([`081a17a`](https://github.com/dc-tec/openbao-operator/commit/081a17a02060db9fb188477620fbd76d1c55522e))
- structured configuration ([`503961d`](https://github.com/dc-tec/openbao-operator/commit/503961d790f996e1de193b321d005d2d8dcc0d4d))
- admission validation policies; backup auth ([`a76541d`](https://github.com/dc-tec/openbao-operator/commit/a76541d5d6770838f69d8b203ff2b4785b23c7a5))
- security; rbac; backup and upgrade improvements ([`89a5ee9`](https://github.com/dc-tec/openbao-operator/commit/89a5ee9e1f2f95ab4c374f7831e48167a4b1303b))

#### Fixes

- secure defaults and profiles ([`6617383`](https://github.com/dc-tec/openbao-operator/commit/66173839968834008119e07cf29cc99188ef8121))
- make JWT auth bootstrap a opt-in feature ([`ded02a3`](https://github.com/dc-tec/openbao-operator/commit/ded02a3173c672e7cbc03f5e993635e1cb345663))
- operator namespace detection ([`139450a`](https://github.com/dc-tec/openbao-operator/commit/139450a401beb7b292aad7962e84e6dcfb109098))
- improve operator rbac ([`8a17db3`](https://github.com/dc-tec/openbao-operator/commit/8a17db3dc103b294e7b602c0a69ff1088e5393e7))
- rbac; upgrade deps ([`8b7d4e8`](https://github.com/dc-tec/openbao-operator/commit/8b7d4e85c080473fcc01d53e103ffc3891d0949a))

#### Tests

- install manifest ([`ec2fb36`](https://github.com/dc-tec/openbao-operator/commit/ec2fb362aecd38bb835b1db2c704378aa5bb7750))

#### Chores

- install manifest ([`465b5a8`](https://github.com/dc-tec/openbao-operator/commit/465b5a8bc4008cf506c35a3baa6d3c4e6da41e7b))
- update configuration ([`5c71c56`](https://github.com/dc-tec/openbao-operator/commit/5c71c5605f8fdb80bb57a39c6063a400cf396f56))
- resolve trivy findings ([`4514a0f`](https://github.com/dc-tec/openbao-operator/commit/4514a0fb964bcc08bd85b486d64f2f2a30fef6dc))
- update manifests ([`10a32ab`](https://github.com/dc-tec/openbao-operator/commit/10a32ab9fa61817da9b4ca5863768722f2b6a1f7))
- kubebuilder scaffolding ([`d768cec`](https://github.com/dc-tec/openbao-operator/commit/d768cecf92a504f3cb7888d7e525d3e8f6e2e844))

### e2e

#### Features

- end-to-end testing ([`47bed1f`](https://github.com/dc-tec/openbao-operator/commit/47bed1fa19b3beadf1cbb339a16456aa0b519359))

#### Fixes

- sentinel drift detection robustness ([`648f3df`](https://github.com/dc-tec/openbao-operator/commit/648f3df3b08e71633f172993a22ecaf0559acfdb))
- unused param ([`b7a9c02`](https://github.com/dc-tec/openbao-operator/commit/b7a9c0294172e5d52c048e117a874239ffb6d10a))

#### Refactors

- migrate to correct namespace ([`0639eac`](https://github.com/dc-tec/openbao-operator/commit/0639eac5be697644ebe75dc01b8e70fc6cdfb51c))

#### Tests

- realign end to end tests ([`7482e30`](https://github.com/dc-tec/openbao-operator/commit/7482e309490c45aa41e421c647e03114446895f5))
- update e2e tests ([`af9cfc3`](https://github.com/dc-tec/openbao-operator/commit/af9cfc3e6726e22ceca249048c7e126c3cd1d618))
- realign end to end tests ([`2b70fbc`](https://github.com/dc-tec/openbao-operator/commit/2b70fbcd64c202e0eb281ff0936c37d6d1024f85))
- prevent race condition backup/restore tests relying on rustfs ([`f703a3e`](https://github.com/dc-tec/openbao-operator/commit/f703a3e9db41188b1f0d9490c45b502a228c8772))
- e2e test improvements/refactor ([`551eb99`](https://github.com/dc-tec/openbao-operator/commit/551eb99b5262f62befed001f8749faa320d75858))
- improve blue/green upgrade tests ([`3522e73`](https://github.com/dc-tec/openbao-operator/commit/3522e73c92845328f523ad4dd7e30908581a9c81))
- remove uneeded test step ([`c98080e`](https://github.com/dc-tec/openbao-operator/commit/c98080e0a345a4bdae85d970d47e9dc3a1faab3a))
- backup and upgrade tests; split chaos and security tests ([`a7bea87`](https://github.com/dc-tec/openbao-operator/commit/a7bea8777d76c1f10389171c2c7f1ea388a7e3ac))
- paralize e2e test suite ([`e4cad46`](https://github.com/dc-tec/openbao-operator/commit/e4cad46a0d9d329e693e9a8cd9710e245c1b7d6e))
- restructure e2e test suite ([`5f9261d`](https://github.com/dc-tec/openbao-operator/commit/5f9261d6aac889b8ba5bc65fe514ae1338abb4f6))

### backup

#### Fixes

- upgrade paths ([`e2bb9b5`](https://github.com/dc-tec/openbao-operator/commit/e2bb9b5ceded236632ce89eee43a001efc0dca70))
- manual / scheduled backups ([`f68172e`](https://github.com/dc-tec/openbao-operator/commit/f68172e4800ce383d8a5b40e910465f6ad1ce86c))
- backup ([`8bdc5fa`](https://github.com/dc-tec/openbao-operator/commit/8bdc5fa0f00affcc6ca8c172f64bdb557a994b54))
- pod security context hardening for init and backup containers ([`cec43e6`](https://github.com/dc-tec/openbao-operator/commit/cec43e6c7fa1e080ad4ec4d223bcb61d2106bbf2))

#### Refactors

- backp manager improvements ([`7991a3e`](https://github.com/dc-tec/openbao-operator/commit/7991a3e80cc02fb3d89d163332d0984c2f0ff4f5))
- improved controller entrypoint; health probe; backup; sentinel; wrapper and main entry ([`1dc089f`](https://github.com/dc-tec/openbao-operator/commit/1dc089fccb64579c15edbf2a2ba007b34e5a29b8))

#### Chores

- update mkdocs reference compat matrix ([`d4dfaf4`](https://github.com/dc-tec/openbao-operator/commit/d4dfaf43a0e33bea5d4558e2de498c3bd40e054b))
- resolve gosec findings ([`06cde7b`](https://github.com/dc-tec/openbao-operator/commit/06cde7be30bdfd51e40303a32daa89801a4216d4))

### user-guide

#### Documentation

- documentation refactor ([`8e7cced`](https://github.com/dc-tec/openbao-operator/commit/8e7cced7e826219a12e44b2d487ba87af22e2f05))
- refactor documentation ([`dde713e`](https://github.com/dc-tec/openbao-operator/commit/dde713e85a2df4884de523631380d683f0e2f17d))
- refactor documentation ([`394b52c`](https://github.com/dc-tec/openbao-operator/commit/394b52ceec6b12a5b379737ca103c1f17817d5bc))
- realign docs ([`6b3521e`](https://github.com/dc-tec/openbao-operator/commit/6b3521ed91204c88e00fc577419eeccd4208adb2))
- realign documentation with codebase ([`f0a360f`](https://github.com/dc-tec/openbao-operator/commit/f0a360f5a1f09f2b2537bbc94a3f51adf0398d3b))
- update documentation ([`5c00ace`](https://github.com/dc-tec/openbao-operator/commit/5c00ace34c4c1362407133f6da185f6599ec0e98))
- update docs to reflect structured configuration ([`cd456f3`](https://github.com/dc-tec/openbao-operator/commit/cd456f344fed62817045342c5f02b556f880c193))
- refactor documentation ([`ad6233d`](https://github.com/dc-tec/openbao-operator/commit/ad6233dcf670a9f1bb820caef90bbae6cdfa1b52))

### build

#### CI

- add helpers for catching config drift upstream ([`5251beb`](https://github.com/dc-tec/openbao-operator/commit/5251bebe916ea6a6bcd1d773ec691bc78878b5ca))

#### Chores

- update Makefile ([`c045039`](https://github.com/dc-tec/openbao-operator/commit/c0450391437fc94cdf6ab9232375dc1ed887dd88))
- maintenance helpers ([`0d68e90`](https://github.com/dc-tec/openbao-operator/commit/0d68e9064dc067b252d696d1b700ffe8407007e1))
- update Makefile ([`ed80782`](https://github.com/dc-tec/openbao-operator/commit/ed8078282d4fe63cf9d85c11af6a78a37860a5e4))
- by default use 2 parallel nodes ([`de68046`](https://github.com/dc-tec/openbao-operator/commit/de680467b7978df60545585d2f393356f841e52b))
- update makefile to support running e2e tests based on labels ([`934a88b`](https://github.com/dc-tec/openbao-operator/commit/934a88b15b72b003ba05c97153f7a6a078e1c705))
- update Makefile so targeted e2e tests can run; run tests in parallel ([`dbabe60`](https://github.com/dc-tec/openbao-operator/commit/dbabe60aa4eb13ae9853e8f4213337063ad6eed6))

### controller

#### Breaking Changes

- openbaocluster refactor; sentinel improvements ([`9d0de98`](https://github.com/dc-tec/openbao-operator/commit/9d0de984d9681d53f4c5569ff84443ae46e2bad5))

#### Features

- single tenancy support ([`b7338d6`](https://github.com/dc-tec/openbao-operator/commit/b7338d623db29590a4f8d0d4427b049f73a86ae8))
- improve event filtering using centralized predicates ([`968df6c`](https://github.com/dc-tec/openbao-operator/commit/968df6c7c58cd7fb95793208605c4ae2f8fe4e8d))

#### Fixes

- timeout for image verification ([`cbcd9cf`](https://github.com/dc-tec/openbao-operator/commit/cbcd9cf753ee6b33d0167d8195cfff69a13e966c))
- persist initialized status ([`c2ebbd1`](https://github.com/dc-tec/openbao-operator/commit/c2ebbd1b6701982fbf5881d71c5a073d35f9854d))
- strengthen status updates with patching ([`6c54a5e`](https://github.com/dc-tec/openbao-operator/commit/6c54a5e4505c8ff17756d8cc477b75641eaebedc))

#### Refactors

- operator controller refactor ([`5c9a452`](https://github.com/dc-tec/openbao-operator/commit/5c9a452d55ffbf8121d3b167a5ffdf96b58a85f8))

### docs

#### Documentation

- realign documentation ([`c4a0d89`](https://github.com/dc-tec/openbao-operator/commit/c4a0d89430f353d1507e874796ca3fd6d974be7d))
- remove non relevant non-goals ([`87c0000`](https://github.com/dc-tec/openbao-operator/commit/87c0000a829c71eb189c09452706cc52b95b3bf5))
- update documentation structure; mkdocs support ([`e060e67`](https://github.com/dc-tec/openbao-operator/commit/e060e67dd152f7678dc2d97f136b352d83d971a4))
- update documentation blue/green ([`3f1f266`](https://github.com/dc-tec/openbao-operator/commit/3f1f2664064b9ae02ab21182794514bfa2a738ee))
- update documentation ([`a6acbef`](https://github.com/dc-tec/openbao-operator/commit/a6acbef86cdd78048ce3c55c84c1c25cf794b5ce))
- add security model docs ([`a932e44`](https://github.com/dc-tec/openbao-operator/commit/a932e4454e29fab4cefff7d1ef960c965577edbc))

#### Chores

- update docs ([`9f4370f`](https://github.com/dc-tec/openbao-operator/commit/9f4370f20a868dbdedaf9d3285d385bb3d09d859))

### misc

#### Refactors

- provision binary ([`e935c8d`](https://github.com/dc-tec/openbao-operator/commit/e935c8d0687f4808e7234fb4bb29e47b13c5bdf8))

#### Chores

- update gitignore ([`836dea8`](https://github.com/dc-tec/openbao-operator/commit/836dea8ba1ea67ddc6de12b02b145015daee7aea))
- add missing restore doc ([`49c9271`](https://github.com/dc-tec/openbao-operator/commit/49c9271ef0213a3305390bae65df57b1b149de07))
- update gitignore ([`255ee32`](https://github.com/dc-tec/openbao-operator/commit/255ee32c1956f63070f5f972479433dcaafd753a))
- realign standards document ([`ca86055`](https://github.com/dc-tec/openbao-operator/commit/ca86055959da42217841618fb1511c4a4f44aaa3))
- update .gitignore ([`82205ca`](https://github.com/dc-tec/openbao-operator/commit/82205cab63c194aed3bb4b60474077dfbced78d9))
- add docs ([`27def4b`](https://github.com/dc-tec/openbao-operator/commit/27def4b6ba39c91fb421e1dd3d6ae05096214fd6))

### infra

#### Features

- standardize sub reconciler pattern ([`ae79ef5`](https://github.com/dc-tec/openbao-operator/commit/ae79ef5d82cf3c2510b9e23150639d35d73a810a))
- operator security hardening ([`34e703f`](https://github.com/dc-tec/openbao-operator/commit/34e703fc57d8915a68c8683f2cd34006a0316505))

#### Refactors

- infra manager ([`1c970ca`](https://github.com/dc-tec/openbao-operator/commit/1c970ca03edf33b576b9fd28e51e55c08281b77d))

#### Tests

- realign sentinel policy tests ([`db9f0b1`](https://github.com/dc-tec/openbao-operator/commit/db9f0b1d3dac47264b342cb1f60f969e1b960465))
- use structured config for audit devices ([`fcc914f`](https://github.com/dc-tec/openbao-operator/commit/fcc914f13e6cf28db3a3e67364da466141dfce74))

### ai

#### Chores

- update rules ([`e3363c8`](https://github.com/dc-tec/openbao-operator/commit/e3363c81ca2be451b6d77538147703681e582057))
- update project agent rules ([`731db46`](https://github.com/dc-tec/openbao-operator/commit/731db46d2de1a2244e93234aaa4cf3465d7ff322))
- update agent test workflow ([`3612ae8`](https://github.com/dc-tec/openbao-operator/commit/3612ae811a22e80fd74b692ea002ceaa026d500b))
- agent rules and workflows ([`903ee08`](https://github.com/dc-tec/openbao-operator/commit/903ee0879d466442cc58f1eecf7ee608294b621b))

### config

#### Breaking Changes

- openbaocluster config renderer ([`a230262`](https://github.com/dc-tec/openbao-operator/commit/a230262c4795566c21ad58a65b74364e7cdd36b6))

#### Features

- structured config for self-init ([`abf2259`](https://github.com/dc-tec/openbao-operator/commit/abf22590241d1a559bdba857440f0918760a78a4))

#### Refactors

- config rendering ([`e6a9ed8`](https://github.com/dc-tec/openbao-operator/commit/e6a9ed80d61b9877d018cf984f1099c36956e310))

#### Tests

- generate "golden" config files for easier testing / inspection of rendered configs ([`a05b0df`](https://github.com/dc-tec/openbao-operator/commit/a05b0df2fa09a51208205184f3efcd2e23ef3085))

### upgrade

#### Breaking Changes

- upgrade manager; blue/green upgrades ([`2ba56a4`](https://github.com/dc-tec/openbao-operator/commit/2ba56a426caa12a79a069700b0b2a4ede44156e1))

#### Features

- harden backup and restore flows ([`cb542ab`](https://github.com/dc-tec/openbao-operator/commit/cb542ab466e29ddbbf61460ebd9368891aa9e359))

#### Fixes

- add metrics for upgrade ([`936d71e`](https://github.com/dc-tec/openbao-operator/commit/936d71edca1f111a40c8a04bd32910459c24fc93))
- use SSA for upgrade manager ([`d0c289c`](https://github.com/dc-tec/openbao-operator/commit/d0c289ce76686f7329e79cbbbfdc29b172446c74))

### charts

#### Features

- operator helm chart ([`c00ff58`](https://github.com/dc-tec/openbao-operator/commit/c00ff58ab1d39b64919acad5456ae221c8b69fc1))

#### Chores

- regenerate helm ([`2a2823b`](https://github.com/dc-tec/openbao-operator/commit/2a2823b16fbdf44c19a09a6f89b9b029532ad2d9))
- helm sync ([`9ccd942`](https://github.com/dc-tec/openbao-operator/commit/9ccd94212ae13688f721ac34a1be38c9c1b0d643))

### ci

#### CI

- update workflows ([`5fb108b`](https://github.com/dc-tec/openbao-operator/commit/5fb108b2d31e0cb3a6c3e5486d0f6e021a43efff))
- sdlc implementation ([`b98f542`](https://github.com/dc-tec/openbao-operator/commit/b98f54267e12cd42aa6e642d59edc503ba81617e))

#### Chores

- templates ([`6585a1b`](https://github.com/dc-tec/openbao-operator/commit/6585a1b3f1875b50d6d10f68d12ebba251d345d9))

### test

#### Features

- tlsroute; monitoring; backup/upgrade ([`bc8497a`](https://github.com/dc-tec/openbao-operator/commit/bc8497a112177ecf837ad3c935af4c90945caafb))

#### Tests

- update test cluster manifests ([`63a1b74`](https://github.com/dc-tec/openbao-operator/commit/63a1b7441120a9529b6493bd324c1d0cdbeb83fe))

#### Chores

- vendor gateway-api ([`c638bee`](https://github.com/dc-tec/openbao-operator/commit/c638bee35136665450a09a82a60d0f0d5ce34bc5))

### admission

#### Fixes

- add admission check ([`50d3af0`](https://github.com/dc-tec/openbao-operator/commit/50d3af0aa06773e5ea5ee98a1194cba7c9f98b1e))
- implement security/rbac improvements ([`95cd1b2`](https://github.com/dc-tec/openbao-operator/commit/95cd1b246c2eacb18e9fa8da977a44ee7faf1313))

### api

#### Features

- improve sentinel observability ([`b9d4168`](https://github.com/dc-tec/openbao-operator/commit/b9d41686964165291a974d900d9050d8be8983c0))

#### Refactors

- remove unused types (2.5.0 gate) ([`6006ef2`](https://github.com/dc-tec/openbao-operator/commit/6006ef2248e8c4d46d648013cb22875468525727))

### bluegreen

#### Features

- blue/green traffic switching improvements ([`5e5f815`](https://github.com/dc-tec/openbao-operator/commit/5e5f8157e52dd7dfcacd07565cd35270c0ec3f20))

#### Refactors

- upgrade resilience improvements ([`8e4d135`](https://github.com/dc-tec/openbao-operator/commit/8e4d135ebc61ea5a4b932be28cf6cea4726bc9c6))

### contributing

#### Documentation

- add conventional commits doc ([`41449d9`](https://github.com/dc-tec/openbao-operator/commit/41449d9760b7cea9f0d678b838955c0f9e7affea))
- update testing documentation ([`868bc1f`](https://github.com/dc-tec/openbao-operator/commit/868bc1f81d1abaca7fbb7a2ba68239dba57411a1))

### security

#### Features

- cosign keyless image verification ([`0c60a60`](https://github.com/dc-tec/openbao-operator/commit/0c60a60a530904695241707aba96cae7edf8390f))

#### Documentation

- restructure documentation ([`f41a235`](https://github.com/dc-tec/openbao-operator/commit/f41a23531e1e7dc675c053c34e00cce44474657d))

### sentinel

#### Fixes

- prevent noisy neighbors and thundering herd behavior ([`57eb7bd`](https://github.com/dc-tec/openbao-operator/commit/57eb7bdfd9b714e2d64c0954d5a36c260dde7efa))

#### Refactors

- sentinel binary ([`a056e6c`](https://github.com/dc-tec/openbao-operator/commit/a056e6cc64adc8bce34fae322d647b639430e1ef))

### deps

#### Chores

- deps ([`7b54fac`](https://github.com/dc-tec/openbao-operator/commit/7b54fac56b38e02c66c7cc171178884acac2fd08))

### integration

#### Tests

- integration tests ([`0ffe575`](https://github.com/dc-tec/openbao-operator/commit/0ffe575c6973ac07b24f24a2e5e178f7eb126d81))

### kube

#### Fixes

- add job check ([`a7439a9`](https://github.com/dc-tec/openbao-operator/commit/a7439a9fe060a4710deda76bea6b7bfafde18020))

### restore

#### Refactors

- restore handeling ([`eedfc4e`](https://github.com/dc-tec/openbao-operator/commit/eedfc4e33807bf60c91666db212308884b729115))

