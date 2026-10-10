#!/usr/bin/env node
// `pnpm check:public`: scans the built public site (dist/) for anything that
// must not be published. Run it after `pnpm build:public`.
//
// Every file in dist/ is scanned, whatever its type. A file that decompresses
// (gzip, zlib/deflate, brotli) is scanned in its decompressed form too, so the
// compressed search index is covered. The scan is case-insensitive over the
// bytes (as latin1), so UTF-8 names are found; it does not look for UTF-16.
//
// Exits 1 and lists every hit (file, term, context) if anything is found.
import { existsSync, readFileSync, readdirSync } from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";
import { brotliDecompressSync, gunzipSync, inflateRawSync, inflateSync } from "node:zlib";

const site = path.resolve(path.dirname(fileURLToPath(import.meta.url)), "..");
const dist = path.join(site, "dist");

// Names that must never appear in the public site. The names of the redaction
// mapping are added below, so extending public-redactions.json extends this list.
const DENYLIST = [
  "arcteryx-order-management-system",
  "order-mgmt",
  "kuzma-wms",
  "kuzma",
  "arcteryx-console",
  "arcteryx-jp",
  "arcteryx",
  "VectorSprint",
  "realworld-eval",
  "/root/",
  "workspace",
  "auth_admin",
  "idempotency.go",
  "check-tracking",
  "/api/v1/auth",
  "/api/v1/inbound",
  "handlers.go:4",
  "localhost:4321",
  // No URL path may contain the journal.
  "/journal/",
];

// Exact phrases in which a denied term is ordinary wording, not a leak. The
// editor guide uses VS Code's own "workspace" (settings and scope) and docs/
// stays unchanged. A hit is ignored only when one of these phrases sits at
// that spot; any other occurrence of the term still fails. Keep this list short.
const ALLOWED_PHRASES = [
  { term: "workspace", phrase: "user or workspace settings" },
  { term: "workspace", phrase: "workspace scope" },
  { term: "workspace", phrase: "in the workspace. add" },
];

const mapping = JSON.parse(readFileSync(path.join(site, "public-redactions.json"), "utf8"));
const needles = [...new Set([...DENYLIST, ...Object.keys(mapping).filter((k) => !k.startsWith("_"))])];

if (!existsSync(dist)) {
  process.stderr.write("check:public: dist/ does not exist. Run `pnpm build:public` first.\n");
  process.exit(2);
}

function* walk(dir) {
  for (const entry of readdirSync(dir, { withFileTypes: true })) {
    const full = path.join(dir, entry.name);
    if (entry.isDirectory()) yield* walk(full);
    else yield full;
  }
}

// Each decoder returns the decompressed bytes, or throws when the data is not that format.
const DECODERS = [
  ["gzip", gunzipSync],
  ["zlib", inflateSync],
  ["deflate-raw", inflateRawSync],
  ["brotli", brotliDecompressSync],
];

function views(buf) {
  const out = [["raw", buf]];
  for (const [label, decode] of DECODERS) {
    try {
      const decoded = decode(buf);
      if (decoded.length > 0) out.push([label, decoded]);
    } catch {
      // Not this format.
    }
  }
  return out;
}

/** True when an allowed phrase for `term` starts so that the term at `at` is inside it. */
function isAllowed(lower, term, at) {
  return ALLOWED_PHRASES.some(({ term: t, phrase }) => {
    if (t.toLowerCase() !== term) return false;
    const start = at - phrase.indexOf(t);
    return start >= 0 && lower.startsWith(phrase, start);
  });
}

const printable = (s) => s.replace(/[\r\n\t]/g, " ").replace(/[^\x20-\x7e]/g, ".");

const hits = [];
let files = 0;
let bytes = 0;
for (const file of walk(dist)) {
  files += 1;
  const buf = readFileSync(file);
  bytes += buf.length;
  for (const [view, data] of views(buf)) {
    const text = data.toString("latin1");
    const lower = text.toLowerCase();
    for (const needle of needles) {
      const n = needle.toLowerCase();
      let at = lower.indexOf(n);
      while (at !== -1) {
        if (isAllowed(lower, n, at)) {
          at = lower.indexOf(n, at + n.length);
          continue;
        }
        const context = printable(text.slice(Math.max(0, at - 60), at + needle.length + 60));
        hits.push({ file: path.relative(site, file), view, needle, context });
        at = lower.indexOf(n, at + n.length);
      }
    }
  }
}

if (hits.length > 0) {
  process.stderr.write(`check:public: FAILED, ${hits.length} hit(s) in ${files} file(s).\n`);
  const byTerm = new Map();
  for (const h of hits) byTerm.set(h.needle, (byTerm.get(h.needle) ?? 0) + 1);
  for (const [term, count] of byTerm) process.stderr.write(`  "${term}": ${count}\n`);
  for (const h of hits) {
    process.stderr.write(`  ${h.file} [${h.view}] "${h.needle}": ...${h.context}...\n`);
  }
  process.exit(1);
}
process.stdout.write(
  `check:public: OK. Scanned ${files} file(s), ${bytes} bytes, with ${needles.length} denied terms. 0 hits.\n`,
);
