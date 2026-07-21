import {spawnSync} from 'node:child_process';
import fs from 'node:fs/promises';
import path from 'node:path';

const version = process.argv[2];
const websiteRoot = process.cwd();
const versionsPath = path.join(websiteRoot, 'versions.json');

if (!version) {
  console.error('Usage: pnpm run refresh:docs-version <version>');
  process.exit(1);
}

function isStableLineVersion(candidate) {
  return /^\d+\.\d+\.0$/.test(candidate);
}

function run(command, args) {
  const result = spawnSync(command, args, {
    cwd: websiteRoot,
    stdio: 'inherit',
  });

  if (result.status !== 0) {
    throw new Error(`${[command, ...args].join(' ')} failed with status ${result.status ?? 1}`);
  }
}

async function pathExists(target) {
  try {
    await fs.access(target);
    return true;
  } catch (error) {
    if (error?.code === 'ENOENT') {
      return false;
    }
    throw error;
  }
}

async function moveIfExists(from, to) {
  if (await pathExists(from)) {
    await fs.mkdir(path.dirname(to), {recursive: true});
    await fs.rename(from, to);
    return true;
  }
  return false;
}

async function restoreIfMoved(from, to, moved) {
  if (!moved) {
    return;
  }
  await fs.rm(to, {recursive: true, force: true});
  await fs.mkdir(path.dirname(to), {recursive: true});
  await fs.rename(from, to);
}

if (!isStableLineVersion(version)) {
  console.error(
    `Release-line docs refresh only supports stable release-line versions (X.Y.0). Patch releases update the existing release-line snapshot: ${version}`,
  );
  process.exit(1);
}

const raw = await fs.readFile(versionsPath, 'utf8');
const originalVersions = JSON.parse(raw);

if (!Array.isArray(originalVersions)) {
  throw new Error('versions.json is not an array');
}

const dedupedOriginalVersions = [...new Set(originalVersions)];
if (!dedupedOriginalVersions.includes(version)) {
  console.error(
    `Docs version ${version} is not present in versions.json. Create the release-line snapshot first with: make docs-version DOCS_VERSION=${version}`,
  );
  process.exit(1);
}

const tempRoot = path.join(websiteRoot, `.tmp-refresh-version-${process.pid}`);
const versionedDocsDir = path.join(websiteRoot, 'versioned_docs', `version-${version}`);
const versionedSidebarPath = path.join(
  websiteRoot,
  'versioned_sidebars',
  `version-${version}-sidebars.json`,
);
const backupDocsDir = path.join(tempRoot, 'versioned_docs', `version-${version}`);
const backupSidebarPath = path.join(
  tempRoot,
  'versioned_sidebars',
  `version-${version}-sidebars.json`,
);

let movedDocs = false;
let movedSidebar = false;

try {
  await fs.rm(tempRoot, {recursive: true, force: true});
  movedDocs = await moveIfExists(versionedDocsDir, backupDocsDir);
  movedSidebar = await moveIfExists(versionedSidebarPath, backupSidebarPath);
  await fs.writeFile(
    versionsPath,
    `${JSON.stringify(dedupedOriginalVersions.filter((candidate) => candidate !== version), null, 2)}\n`,
  );

  run(process.platform === 'win32' ? 'pnpm.cmd' : 'pnpm', ['run', 'prepare:contribute']);
  run(process.platform === 'win32' ? 'pnpm.cmd' : 'pnpm', [
    'exec',
    'docusaurus',
    'docs:version',
    version,
  ]);

  await fs.writeFile(versionsPath, `${JSON.stringify(dedupedOriginalVersions, null, 2)}\n`);
  await fs.rm(tempRoot, {recursive: true, force: true});
} catch (error) {
  await fs.rm(versionedDocsDir, {recursive: true, force: true});
  await fs.rm(versionedSidebarPath, {force: true});
  await restoreIfMoved(backupDocsDir, versionedDocsDir, movedDocs);
  await restoreIfMoved(backupSidebarPath, versionedSidebarPath, movedSidebar);
  await fs.writeFile(versionsPath, `${JSON.stringify(dedupedOriginalVersions, null, 2)}\n`);
  await fs.rm(tempRoot, {recursive: true, force: true});
  console.error(error.message);
  process.exit(1);
}
