import fs from 'node:fs/promises';
import path from 'node:path';

const rootDir = path.resolve(process.cwd(), '..');
const changelogPath = path.join(rootDir, 'CHANGELOG.md');
const releaseNotesDir = path.join(rootDir, 'release-notes');
const releasesDir = path.join(rootDir, 'releases');
const versionsPath = path.join(process.cwd(), 'versions.json');

function slugForVersion(version) {
  return version.toLowerCase();
}

function isPrerelease(version) {
  return version.includes('-');
}

function isStableVersion(version) {
  return /^\d+\.\d+\.\d+$/.test(version);
}

function docsSnapshotVersionFor(version) {
  const [major, minor] = version.split('.');
  return `${major}.${minor}.0`;
}

async function loadChangelog() {
  return fs.readFile(changelogPath, 'utf8');
}

async function loadDocsVersions() {
  try {
    const raw = await fs.readFile(versionsPath, 'utf8');
    const versions = JSON.parse(raw);
    return Array.isArray(versions) ? versions : [];
  } catch {
    return [];
  }
}

async function loadManualReleaseNote(slug) {
  for (const extension of ['mdx', 'md']) {
    try {
      return (await fs.readFile(path.join(releaseNotesDir, `${slug}.${extension}`), 'utf8')).trim();
    } catch (error) {
      if (error?.code !== 'ENOENT') {
        throw error;
      }
    }
  }

  return '';
}

function getDocsUrl(version, docsVersions) {
  if (version === 'Unreleased') {
    return '/docs/next';
  }

  if (isPrerelease(version)) {
    return null;
  }

  if (!isStableVersion(version)) {
    return null;
  }

  const docsVersion = docsSnapshotVersionFor(version);
  if (!docsVersions.includes(docsVersion)) {
    return null;
  }

  return docsVersions[0] === docsVersion ? '/docs' : `/docs/${slugForVersion(docsVersion)}`;
}

function parseSections(changelog) {
  const lines = changelog.split('\n');
  const sections = [];
  let current = null;

  for (const line of lines) {
    const headingMatch =
      line.match(/^## \[([^\]]+)\]\(([^)]+)\) \(([^)]+)\)$/) ||
      line.match(/^## ([^(\n]+) \(([^)]+)\)$/) ||
      line.match(/^## (Unreleased)$/);

    if (headingMatch) {
      if (current) {
        sections.push(current);
      }

      if (headingMatch[1] === 'Unreleased') {
        current = {
          version: 'Unreleased',
          slug: 'unreleased',
          title: 'Unreleased',
          date: '',
          compareUrl: '',
          body: [],
        };
        continue;
      }

      if (headingMatch.length === 4) {
        current = {
          version: headingMatch[1].trim(),
          slug: slugForVersion(headingMatch[1].trim()),
          title: headingMatch[1].trim(),
          date: headingMatch[3]?.trim() || headingMatch[2]?.trim() || '',
          compareUrl: headingMatch[2].startsWith('http') ? headingMatch[2] : '',
          body: [],
        };
        continue;
      }

      current = {
        version: headingMatch[1].trim(),
        slug: slugForVersion(headingMatch[1].trim()),
        title: headingMatch[1].trim(),
        date: headingMatch[2]?.trim() || '',
        compareUrl: '',
        body: [],
      };
      continue;
    }

    if (current) {
      current.body.push(line);
    }
  }

  if (current) {
    sections.push(current);
  }

  return sections;
}

async function writeReleaseIndex(sections) {
  const latest = sections.find((section) => section.version !== 'Unreleased') ?? sections[0];
  const archive = sections
    .filter((section) => section.version !== 'Unreleased')
    .map(
      (section) =>
        `- [${section.title}](/releases/${section.slug})${
          section.date ? ` — ${section.date}` : ''
        }`,
    )
    .join('\n');

  const content = `---
title: Release Notes
description: Version history and release highlights for OpenBao Operator.
slug: /
---

# Release Notes

<StatusPill>${latest?.version ?? 'Current release'}</StatusPill>

Release pages combine hand-written notes from [release-notes/](https://github.com/dc-tec/openbao-operator/tree/main/release-notes) with generated entries from [CHANGELOG.md](https://github.com/dc-tec/openbao-operator/blob/main/CHANGELOG.md).

## Latest highlighted release

<CardGrid>
  <LinkCard title="${latest?.title ?? 'Current'}" to="/releases/${latest?.slug ?? 'unreleased'}">
    ${latest?.date ? `Published ${latest.date}.` : 'See the current release status.'} Open the full notes, compare changes, and jump into the matching docs version.
  </LinkCard>
  <LinkCard title="GitHub Releases" to="https://github.com/dc-tec/openbao-operator/releases">
    Browse published release assets, tags, and signed artifacts in GitHub.
  </LinkCard>
</CardGrid>

## Archive

${archive}
`;

  await fs.writeFile(path.join(releasesDir, 'index.mdx'), content);
}

async function writeReleasePages(sections, docsVersions) {
  for (const section of sections) {
    const body = section.body.join('\n').trim();
    const manualReleaseNote = await loadManualReleaseNote(section.slug);
    const githubReleaseUrl =
      section.version === 'Unreleased'
        ? 'https://github.com/dc-tec/openbao-operator/pulls?q=is%3Apr+is%3Aopen+label%3Arelease'
        : `https://github.com/dc-tec/openbao-operator/releases/tag/${section.version}`;
    const docsUrl = getDocsUrl(section.version, docsVersions);
    const hasDocsSnapshot = docsUrl !== null;
    const docsCard =
      section.version === 'Unreleased'
        ? `<LinkCard title="Matching docs" to="${docsUrl}">
    Open the docs experience aligned to unreleased changes on main.
  </LinkCard>`
        : hasDocsSnapshot
          ? `<LinkCard title="Matching docs" to="${docsUrl}">
    Open the docs experience aligned to this release line.
  </LinkCard>`
          : `<Callout type="note" title="Docs snapshot unavailable">

This release note is archived, but a matching versioned docs snapshot is not published in this site yet.

</Callout>`;

    const content = `---
title: ${section.title}
description: Release notes for OpenBao Operator ${section.title}.
slug: /${section.slug}
---

<StatusPill>${section.version === 'Unreleased' ? 'Next Release' : section.title}</StatusPill>

${section.date ? `Published ${section.date}.` : 'This page tracks unreleased changes on main.'}

<CardGrid>
  ${docsCard}
  <LinkCard title="GitHub release" to="${githubReleaseUrl}">
    View release assets, tag metadata, and the GitHub release entry.
  </LinkCard>
</CardGrid>

${manualReleaseNote ? `${manualReleaseNote}\n\n` : ''}${body}
`;

    await fs.writeFile(path.join(releasesDir, `${section.slug}.mdx`), content);
  }
}

async function main() {
  const changelog = await loadChangelog();
  const docsVersions = await loadDocsVersions();
  const sections = parseSections(changelog);

  await fs.rm(releasesDir, {recursive: true, force: true});
  await fs.mkdir(releasesDir, {recursive: true});
  await writeReleaseIndex(sections);
  await writeReleasePages(sections, docsVersions);
}

main().catch((error) => {
  console.error(error);
  process.exitCode = 1;
});
