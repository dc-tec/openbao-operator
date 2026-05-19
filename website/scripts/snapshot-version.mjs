import {spawnSync} from 'node:child_process';
import fs from 'node:fs/promises';
import path from 'node:path';

const version = process.argv[2];
const versionsPath = path.join(process.cwd(), 'versions.json');

if (!version) {
  console.error('Usage: npm run version:docs -- <version>');
  process.exit(1);
}

function isStableLineVersion(candidate) {
  return /^\d+\.\d+\.0$/.test(candidate);
}

if (!isStableLineVersion(version)) {
  console.error(
    `Docs snapshots are only published for stable release-line versions (X.Y.0). Prereleases and patch releases use release notes plus the release-line docs: ${version}`,
  );
  process.exit(1);
}

async function dedupeVersions() {
  const raw = await fs.readFile(versionsPath, 'utf8');
  const versions = JSON.parse(raw);

  if (!Array.isArray(versions)) {
    throw new Error('versions.json is not an array');
  }

  const deduped = [...new Set(versions)];
  await fs.writeFile(versionsPath, `${JSON.stringify(deduped, null, 2)}\n`);
}

const prepareResult = spawnSync(
  process.platform === 'win32' ? 'npm.cmd' : 'npm',
  ['run', 'prepare:contribute'],
  {
    stdio: 'inherit',
  },
);

if (prepareResult.status !== 0) {
  process.exit(prepareResult.status ?? 1);
}

const result = spawnSync(
  process.platform === 'win32' ? 'npx.cmd' : 'npx',
  ['docusaurus', 'docs:version', version],
  {
    stdio: 'inherit',
  },
);

if (result.status !== 0) {
  process.exit(result.status ?? 1);
}

await dedupeVersions();
