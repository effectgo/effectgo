#!/usr/bin/env node
// `pnpm lint:docs`: runs the Nimbus authoring linter over the repo's docs/.
//
// `nimbus-docs lint` only reads `.mdx` files under src/content, and the docs are
// `.md` files in ../docs. So this script copies the docs the site includes
// (DOCS_PATTERN) into a gitignored mirror, .lint-docs/src/content/docs, with
// `.mdx` names, and runs the linter there. The copy is for linting only. The
// site itself reads ../docs directly (src/content.config.ts).
//
// The build runs first, because the linter reads the rules and route data from
// .nimbus/, which `astro build` writes from astro.config.ts.
import { spawnSync } from "node:child_process";
import { copyFileSync, cpSync, mkdirSync, readdirSync, rmSync, statSync } from "node:fs";
import path from "node:path";
import { fileURLToPath, pathToFileURL } from "node:url";

const site = path.resolve(path.dirname(fileURLToPath(import.meta.url)), "..");
const { DOCS_DIR, DOCS_PATTERN } = await import(pathToFileURL(path.join(site, "src/docs-source.ts")).href);
const docs = path.resolve(site, DOCS_DIR);
const mirror = path.join(site, ".lint-docs");

function fail(message) {
  process.stderr.write(`lint:docs: ${message}\n`);
  process.exit(1);
}

function walk(dir, prefix = "") {
  return readdirSync(path.join(dir, prefix), { withFileTypes: true }).flatMap((entry) => {
    const rel = prefix ? `${prefix}/${entry.name}` : entry.name;
    if (entry.isDirectory()) return walk(dir, rel);
    return statSync(path.join(dir, rel)).isFile() ? [rel] : [];
  });
}

const build = spawnSync("pnpm", ["build"], { cwd: site, stdio: ["ignore", "inherit", "inherit"] });
if (build.status !== 0) fail("the build failed, so there are no lint rules to run");

const included = walk(docs).filter((rel) => rel.endsWith(".md") && path.matchesGlob(rel, DOCS_PATTERN));
if (included.length === 0) fail(`no docs matched ${DOCS_PATTERN} under ${DOCS_DIR}`);

rmSync(mirror, { recursive: true, force: true });
mkdirSync(path.join(mirror, "src/content/docs"), { recursive: true });
cpSync(path.join(site, ".nimbus"), path.join(mirror, ".nimbus"), { recursive: true });
for (const rel of included) {
  const dest = path.join(mirror, "src/content/docs", rel.replace(/\.md$/, ".mdx"));
  mkdirSync(path.dirname(dest), { recursive: true });
  copyFileSync(path.join(docs, rel), dest);
}

const lint = spawnSync(
  process.execPath,
  [path.join(site, "node_modules/@cloudflare/nimbus-docs/dist/cli/index.js"), "lint", ...process.argv.slice(2)],
  { cwd: mirror, stdio: "inherit" },
);
process.exit(lint.status ?? 1);
