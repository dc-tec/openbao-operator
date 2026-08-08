# Legacy route continuity

`legacy_redirects.tsv` preserves every public route from the former documentation site and the initial `/0.4.x/`
surface.

The tab-separated columns are:

1. the legacy route relative to the repository's GitHub Pages base path;
2. the canonical Hugo route; and
3. the source document or disposition that justifies the mapping.

Mappings follow these boundaries:

- unprefixed and `latest` routes resolve to the current unprefixed stable line;
- Docusaurus `0.4.0` and initial Hugo `0.4.x` routes resolve to the unprefixed `0.4.x` contract;
- `docs/next` and `dev` routes remain on the unreleased `/next/` line;
- unsupported `0.1.x`, `0.2.x`, and `0.3.x` snapshots resolve to their release archive rather than a newer runtime
  contract; and
- merged pages resolve to their reviewed successor, while retired environment recipes resolve to the closest durable
  task or documentation hub.

Apply the ledger after Hugo writes its destination:

```sh
./website/scripts/apply-legacy-redirects.sh \
  --destination website/public

./website/scripts/apply-legacy-redirects.sh \
  --destination website/public \
  --check
```

The generator validates every target before writing static redirect pages and overwrites obsolete Hugo aliases when a
version-aware mapping is required. The optional `--legacy-build` audit can recheck the ledger against an externally
retained copy of the final Docusaurus build; normal builds validate the frozen ledger and every current target.
