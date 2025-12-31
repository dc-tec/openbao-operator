# Contributing

Development, testing, and CI guidance lives under `docs/contributing/`.

Start here:

- `docs/contributing/index.md`
- `docs/contributing/development.md`
- `docs/contributing/ci.md`
- `docs/contributing/testing.md`
- `docs/contributing/release-management.md`

## Local Checks (PR-equivalent)

```sh
make lint-config lint
make verify-fmt
make verify-tidy
make verify-generated
make verify-helm
make test-ci
```
