import fs from 'node:fs/promises';
import path from 'node:path';

const version = process.argv[2];
const versionsPath = path.join(process.cwd(), 'versions.json');

function isStableVersion(candidate) {
  return /^\d+\.\d+\.\d+$/.test(candidate);
}

function docsSnapshotVersionFor(candidate) {
  const [major, minor] = candidate.split('.');
  return `${major}.${minor}.0`;
}

if (!version) {
  console.error('Usage: node ./scripts/verify-docs-version.mjs <version>');
  process.exit(1);
}

try {
  const raw = await fs.readFile(versionsPath, 'utf8');
  const versions = JSON.parse(raw);

  if (!Array.isArray(versions)) {
    console.error('Unable to verify website/versions.json: versions.json is not an array');
    process.exit(1);
  }

  const prereleaseVersions = versions.filter((candidate) => candidate.includes('-'));
  if (prereleaseVersions.length > 0) {
    console.error(
      `Prerelease docs snapshots are not published. Remove these entries from website/versions.json: ${prereleaseVersions.join(', ')}`,
    );
    process.exit(1);
  }

  if (!isStableVersion(version)) {
    console.error(
      `Docs snapshots are only verified for stable SemVer releases. Prereleases use /docs/next plus release notes: ${version}`,
    );
    process.exit(1);
  }

  const docsSnapshotVersion = docsSnapshotVersionFor(version);
  if (!versions.includes(docsSnapshotVersion)) {
    console.error(
      `Stable release ${version} does not have a release-line docs snapshot (${docsSnapshotVersion}) in website/versions.json. Snapshot the docs before the first X.Y.0 release with: make docs-version DOCS_VERSION=${docsSnapshotVersion}`,
    );
    process.exit(1);
  }
} catch (error) {
  console.error(`Unable to verify website/versions.json: ${error.message}`);
  process.exit(1);
}
