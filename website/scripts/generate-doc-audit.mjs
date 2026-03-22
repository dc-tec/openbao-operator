import {promises as fs} from 'node:fs';
import {fileURLToPath} from 'node:url';
import path from 'node:path';

const scriptDir = path.dirname(fileURLToPath(import.meta.url));
const websiteRoot = path.resolve(scriptDir, '..');
const repoRoot = path.resolve(websiteRoot, '..');
const outputPath = path.join(repoRoot, 'contribute', 'docs-audit-inventory.md');

const contentRoots = [
  {name: 'Docs', dir: path.join(repoRoot, 'docs')},
  {name: 'Contribute', dir: path.join(repoRoot, 'contribute')},
];

const modernComponentPatterns = [
  '<PageHero',
  '<RouteList',
  '<JourneyRail',
  '<DecisionTable',
  '<CommandBlock',
  '<DiagramFrame',
  '<NextActions',
];

const legacyPatterns = [/grid cards/, /^!!!\s/m, /^\?\?\?\s/m, /^===\s/m];

const entryPointPatterns = [
  /(^|\/)index\.mdx?$/,
  /(^|\/)overview\.md$/,
  /(^|\/)installation\.md$/,
  /(^|\/)getting-started\.md$/,
  /(^|\/)next-steps\.md$/,
  /(^|\/)upgrades\.md$/,
  /(^|\/)backups\.md$/,
  /(^|\/)troubleshooting\.md$/,
  /(^|\/)safe-mode\.md$/,
  /(^|\/)no-leader\.md$/,
  /(^|\/)sealed-cluster\.md$/,
  /(^|\/)failed-rollback\.md$/,
  /(^|\/)restore\.md$/,
];

async function walk(dir) {
  const entries = await fs.readdir(dir, {withFileTypes: true});
  const files = [];

  for (const entry of entries) {
    const fullPath = path.join(dir, entry.name);

    if (entry.isDirectory()) {
      files.push(...(await walk(fullPath)));
      continue;
    }

    if (entry.isFile() && /\.(md|mdx)$/.test(entry.name)) {
      files.push(fullPath);
    }
  }

  return files;
}

function parseFrontmatter(content) {
  const match = content.match(/^---\n([\s\S]*?)\n---\n?/);

  if (!match) {
    return {};
  }

  const fields = {};

  for (const line of match[1].split('\n')) {
    const field = line.match(/^([A-Za-z0-9_-]+):\s*(.+)$/);
    if (!field) {
      continue;
    }

    const [, key, rawValue] = field;
    fields[key] = rawValue.replace(/^['"]|['"]$/g, '');
  }

  return fields;
}

function extractTitle(content, frontmatter, relativePath) {
  if (frontmatter.title) {
    return frontmatter.title;
  }

  const heading = content.match(/^#\s+(.+)$/m);
  if (heading) {
    return heading[1].trim();
  }

  return path.basename(relativePath).replace(/\.(md|mdx)$/, '');
}

function classifySection(relativePath) {
  if (relativePath.startsWith('contribute/')) {
    return 'Contribute';
  }

  if (relativePath === 'docs/index.mdx') {
    return 'Operator Docs';
  }

  if (relativePath.startsWith('docs/user-guide/operator/')) {
    return 'Get Started';
  }

  if (relativePath === 'docs/user-guide/index.mdx') {
    return 'Get Started';
  }

  if (
    relativePath === 'docs/user-guide/openbaocluster/getting-started.md' ||
    relativePath === 'docs/user-guide/openbaocluster/next-steps.md'
  ) {
    return 'Get Started';
  }

  if (
    relativePath.startsWith('docs/user-guide/openbaocluster/configuration/') ||
    relativePath === 'docs/user-guide/openbaocluster/overview.md' ||
    relativePath === 'docs/user-guide/openbaocluster/security-considerations.md'
  ) {
    return 'Configure';
  }

  if (relativePath.startsWith('docs/user-guide/openbaocluster/operations/')) {
    return 'Operate';
  }

  if (
    relativePath.startsWith('docs/user-guide/openbaocluster/recovery/') ||
    relativePath.startsWith('docs/user-guide/openbaorestore/')
  ) {
    return 'Operate / Recovery & Restore';
  }

  if (relativePath.startsWith('docs/user-guide/openbaotenant/')) {
    return 'Identity & Tenancy';
  }

  if (relativePath.startsWith('docs/user-guide/validated-deployments/')) {
    return 'Validated Deployments';
  }

  if (relativePath.startsWith('docs/security/')) {
    return 'Security';
  }

  if (relativePath.startsWith('docs/architecture/')) {
    return 'Architecture';
  }

  if (relativePath.startsWith('docs/reference/')) {
    return 'Reference';
  }

  return 'Other';
}

function inferRewriteLevel(relativePath, frontmatter, content, needs) {
  const isEntryPoint = entryPointPatterns.some((pattern) => pattern.test(relativePath));
  const isHighTrafficSection =
    classifySection(relativePath).startsWith('Get Started') ||
    classifySection(relativePath).startsWith('Configure') ||
    classifySection(relativePath).startsWith('Operate') ||
    classifySection(relativePath) === 'Security';

  if (needs.includes('legacy-layout') || (isEntryPoint && needs.length > 0)) {
    return 'L2';
  }

  if (isHighTrafficSection && (!frontmatter.pageType || !modernComponentPatterns.some((pattern) => content.includes(pattern)))) {
    return 'L2';
  }

  if (needs.length > 0) {
    return 'L1';
  }

  return 'L0';
}

function collectNeeds(relativePath, frontmatter, content) {
  const needs = [];
  const hasModernComponents = modernComponentPatterns.some((pattern) => content.includes(pattern));
  const hasLegacyLayout = legacyPatterns.some((pattern) => pattern.test(content));
  const hasDescription = Boolean(frontmatter.description);
  const isLandingLike = /(^|\/)(index|overview)\.(md|mdx)$/.test(relativePath);

  if (!hasDescription) {
    needs.push('description');
  }

  if (isLandingLike && !frontmatter.pageType) {
    needs.push('pageType');
  }

  if (hasLegacyLayout) {
    needs.push('legacy-layout');
  }

  if (isLandingLike && !hasModernComponents) {
    needs.push('design-system');
  }

  if (!hasModernComponents && !hasLegacyLayout && !relativePath.startsWith('docs/reference/')) {
    needs.push('copy-tightening');
  }

  if (
    relativePath.includes('/recovery/') &&
    frontmatter.journey &&
    frontmatter.journey !== 'operate'
  ) {
    needs.push('journey-alignment');
  }

  return [...new Set(needs)];
}

function summarizeBy(items, key) {
  return items.reduce((accumulator, item) => {
    const value = item[key];
    accumulator[value] = (accumulator[value] || 0) + 1;
    return accumulator;
  }, {});
}

function formatCounts(counts) {
  return Object.entries(counts)
    .sort((a, b) => a[0].localeCompare(b[0]))
    .map(([label, count]) => `- ${label}: ${count}`)
    .join('\n');
}

function sortItems(items) {
  return [...items].sort((a, b) => a.relativePath.localeCompare(b.relativePath));
}

const records = [];

for (const root of contentRoots) {
  const files = await walk(root.dir);

  for (const filePath of files) {
    if (filePath === outputPath) {
      continue;
    }

    const content = await fs.readFile(filePath, 'utf8');
    const relativePath = path.relative(repoRoot, filePath).replaceAll(path.sep, '/');
    const frontmatter = parseFrontmatter(content);
    const needs = collectNeeds(relativePath, frontmatter, content);
    const rewriteLevel = inferRewriteLevel(relativePath, frontmatter, content, needs);

    records.push({
      relativePath,
      section: classifySection(relativePath),
      title: extractTitle(content, frontmatter, relativePath),
      pageType: frontmatter.pageType || 'unset',
      journey: frontmatter.journey || 'unset',
      rewriteLevel,
      needs,
    });
  }
}

const bySection = summarizeBy(records, 'section');
const byRewrite = summarizeBy(records, 'rewriteLevel');

const groupedSections = Object.entries(
  records.reduce((accumulator, record) => {
    accumulator[record.section] ||= [];
    accumulator[record.section].push(record);
    return accumulator;
  }, {}),
)
  .sort((a, b) => a[0].localeCompare(b[0]))
  .map(([section, items]) => {
    const lines = sortItems(items).map((item) => {
      const needsLabel = item.needs.length === 0 ? 'ready' : item.needs.join(', ');
      return `- [ ] ${item.rewriteLevel} — \`${item.relativePath}\` — ${item.title} — pageType: \`${item.pageType}\`, journey: \`${item.journey}\`, needs: ${needsLabel}`;
    });

    return `## ${section}\n\n${lines.join('\n')}`;
  })
  .join('\n\n');

const markdown = `# Docs Audit Inventory

This file is generated by \`npm --prefix website run audit:docs\`. It is the editorial backlog for the Docusaurus migration and design-system rollout.

## Summary

- Total pages: ${records.length}
- Sections tracked: ${Object.keys(bySection).length}

### Rewrite Levels

${formatCounts(byRewrite)}

### Sections

${formatCounts(bySection)}

## Rewrite Levels

- \`L0\`: page is broadly aligned with the current design system and mainly needs incremental cleanup.
- \`L1\`: page needs copy tightening, frontmatter cleanup, or light component migration.
- \`L2\`: page is an entry point or still carries MkDocs-era structure and needs a full editorial/design pass.

## Inventory

${groupedSections}
`;

await fs.writeFile(outputPath, markdown);
console.log(`Wrote ${path.relative(repoRoot, outputPath)} (${records.length} pages)`);
