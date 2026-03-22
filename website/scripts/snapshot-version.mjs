import {spawnSync} from 'node:child_process';
import fs from 'node:fs/promises';
import path from 'node:path';

const version = process.argv[2];
const versionsPath = path.join(process.cwd(), 'versions.json');

if (!version) {
  console.error('Usage: npm run version:docs -- <version>');
  process.exit(1);
}

function isPrerelease(candidate) {
  return candidate.includes('-');
}

async function reorderVersions() {
  const raw = await fs.readFile(versionsPath, 'utf8');
  const versions = JSON.parse(raw);

  if (!Array.isArray(versions)) {
    throw new Error('versions.json is not an array');
  }

  const deduped = [...new Set(versions)];

  if (!isPrerelease(version)) {
    await fs.writeFile(versionsPath, `${JSON.stringify(deduped, null, 2)}\n`);
    return;
  }

  const stableVersions = deduped.filter((candidate) => !isPrerelease(candidate));
  if (stableVersions.length === 0) {
    await fs.writeFile(versionsPath, `${JSON.stringify(deduped, null, 2)}\n`);
    return;
  }

  const reordered = deduped.filter((candidate) => candidate !== version);
  reordered.splice(1, 0, version);
  await fs.writeFile(versionsPath, `${JSON.stringify(reordered, null, 2)}\n`);
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

await reorderVersions();
