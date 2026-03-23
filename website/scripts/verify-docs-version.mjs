import fs from 'node:fs/promises';
import path from 'node:path';

const version = process.argv[2];
const versionsPath = path.join(process.cwd(), 'versions.json');

function isPrerelease(candidate) {
  return candidate.includes('-');
}

if (!version) {
  console.error('Usage: node ./scripts/verify-docs-version.mjs <version>');
  process.exit(1);
}

try {
  const raw = await fs.readFile(versionsPath, 'utf8');
  const versions = JSON.parse(raw);

  if (isPrerelease(version)) {
    if (!Array.isArray(versions)) {
      console.error('Unable to verify website/versions.json: versions.json is not an array');
      process.exit(1);
    }

    if (
      versions.includes(version) &&
      versions[0] === version &&
      versions.some((candidate) => !isPrerelease(candidate))
    ) {
      console.error(
        `Prerelease ${version} is first in website/versions.json, which would move /docs away from the latest stable version. Re-run make docs-version DOCS_VERSION=${version} or fix versions.json ordering before release.`,
      );
      process.exit(1);
    }

    process.exit(0);
  }

  if (!Array.isArray(versions) || !versions.includes(version)) {
    console.error(
      `Version ${version} is not present in website/versions.json. Snapshot the docs before release with: make docs-version DOCS_VERSION=${version}`,
    );
    process.exit(1);
  }
} catch (error) {
  console.error(`Unable to verify website/versions.json: ${error.message}`);
  process.exit(1);
}
