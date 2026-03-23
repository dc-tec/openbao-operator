import {promises as fs} from 'node:fs';
import {fileURLToPath} from 'node:url';
import path from 'node:path';

const scriptDir = path.dirname(fileURLToPath(import.meta.url));
const websiteRoot = path.resolve(scriptDir, '..');
const repoRoot = path.resolve(websiteRoot, '..');
const sourceDir = path.join(repoRoot, 'docs', 'contribute');
const outputDir = path.join(repoRoot, '.contribute-docs');

await fs.rm(outputDir, {recursive: true, force: true});
await fs.cp(sourceDir, outputDir, {recursive: true});

console.log(`Synced ${path.relative(repoRoot, sourceDir)} -> ${path.relative(repoRoot, outputDir)}`);
