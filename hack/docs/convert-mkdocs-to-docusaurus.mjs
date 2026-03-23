import fs from 'node:fs/promises';
import path from 'node:path';

const rootDir = process.cwd();
const targets = ['docs', 'contribute'];

function stripIconShortcodes(input) {
  return input.replace(/:(material|simple)-[a-z0-9-]+:\s*/gi, '');
}

function normalizeInlineSyntax(input) {
  let output = stripIconShortcodes(input);
  output = output.replace(/\]\(([^)]+)\)\{\:target="_blank"\}/g, ']($1)');
  output = output.replace(/\[([^\]]+)\]\{ \.md-button \.md-button--primary \}/g, '<span className="button button--primary">$1</span>');
  output = output.replace(/\[([^\]]+)\]\{ \.md-button \}/g, '<span className="button button--secondary">$1</span>');
  output = output.replace(/^(#{1,6})\s+(.+?)\s+\{\:\s*#([A-Za-z0-9_-]+)\s*\}\s*$/g, (_match, level, title, id) => {
    const depth = String(level).length;
    return `<h${depth} id="${id}">${title}</h${depth}>`;
  });
  return output;
}

function collectIndentedBlock(lines, startIndex, baseIndent) {
  const blockIndent = `${baseIndent}    `;
  const collected = [];
  let index = startIndex;

  while (index < lines.length) {
    const line = lines[index];
    if (line.startsWith(blockIndent)) {
      collected.push(line.slice(blockIndent.length));
      index += 1;
      continue;
    }

    if (line.trim() === '') {
      collected.push('');
      index += 1;
      continue;
    }

    break;
  }

  while (collected.length > 0 && collected[0] === '') {
    collected.shift();
  }
  while (collected.length > 0 && collected[collected.length - 1] === '') {
    collected.pop();
  }

  return {block: collected, nextIndex: index};
}

function tabValue(label) {
  return label
    .toLowerCase()
    .replace(/[^a-z0-9]+/g, '-')
    .replace(/^-|-$/g, '') || 'tab';
}

function tabGroupId(labels) {
  return labels.map(tabValue).join('-');
}

function transformLines(lines) {
  const output = [];

  for (let index = 0; index < lines.length; ) {
    const line = normalizeInlineSyntax(lines[index]);
    const admonitionMatch = line.match(/^(\s*)(!!!|\?\?\?)\s+([A-Za-z_-]+)(?:\s+"([^"]+)")?\s*$/);
    const tabMatch = line.match(/^(\s*)===\s+"(.+)"\s*$/);

    if (admonitionMatch) {
      const [, indent, marker, rawType, rawTitle] = admonitionMatch;
      const {block, nextIndex} = collectIndentedBlock(lines, index + 1, indent);
      const children = transformLines(block);
      const component = marker === '!!!' ? 'Callout' : 'ExpandableCallout';
      const attrs = [`type=${JSON.stringify(rawType.toLowerCase())}`];

      if (rawTitle) {
        attrs.push(`title=${JSON.stringify(stripIconShortcodes(rawTitle).trim())}`);
      }

      output.push(`${indent}<${component} ${attrs.join(' ')}>`);
      if (children.length > 0) {
        output.push('');
        output.push(...children);
        output.push('');
      }
      output.push(`${indent}</${component}>`);
      output.push('');
      index = nextIndex;
      continue;
    }

    if (tabMatch) {
      const [, indent] = tabMatch;
      const tabs = [];
      let cursor = index;

      while (cursor < lines.length) {
        const current = normalizeInlineSyntax(lines[cursor]);
        const currentMatch = current.match(/^(\s*)===\s+"(.+)"\s*$/);
        if (!currentMatch || currentMatch[1] !== indent) {
          break;
        }

        const label = stripIconShortcodes(currentMatch[2]).trim();
        const {block, nextIndex} = collectIndentedBlock(lines, cursor + 1, indent);
        tabs.push({
          label,
          body: transformLines(block),
        });
        cursor = nextIndex;

        while (cursor < lines.length && lines[cursor].trim() === '') {
          cursor += 1;
        }
      }

      output.push(`${indent}<Tabs groupId=${JSON.stringify(tabGroupId(tabs.map((tab) => tab.label)))}>`);
      output.push('');
      for (const tab of tabs) {
        output.push(
          `${indent}<TabItem value=${JSON.stringify(tabValue(tab.label))} label=${JSON.stringify(tab.label)}>`,
        );
        output.push('');
        output.push(...tab.body);
        output.push('');
        output.push(`${indent}</TabItem>`);
        output.push('');
      }
      output.push(`${indent}</Tabs>`);
      output.push('');
      index = cursor;
      continue;
    }

    output.push(line);
    index += 1;
  }

  while (output.length > 1 && output[output.length - 1] === '' && output[output.length - 2] === '') {
    output.pop();
  }

  return output;
}

async function convertFile(filePath) {
  const original = await fs.readFile(filePath, 'utf8');
  const lines = original.replace(/\r\n/g, '\n').split('\n');
  const transformed = `${transformLines(lines).join('\n').replace(/\n{3,}/g, '\n\n')}\n`;

  if (transformed !== original) {
    await fs.writeFile(filePath, transformed);
  }
}

async function visit(dirPath) {
  const entries = await fs.readdir(dirPath, {withFileTypes: true});

  for (const entry of entries) {
    if (entry.name === 'assets' || entry.name === 'stylesheets' || entry.name === 'overrides') {
      continue;
    }

    const fullPath = path.join(dirPath, entry.name);
    if (entry.isDirectory()) {
      await visit(fullPath);
      continue;
    }

    if (entry.isFile() && /\.(md|mdx)$/i.test(entry.name)) {
      await convertFile(fullPath);
    }
  }
}

async function main() {
  for (const target of targets) {
    await visit(path.join(rootDir, target));
  }
}

main().catch((error) => {
  console.error(error);
  process.exitCode = 1;
});
