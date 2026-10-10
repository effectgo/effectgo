// Public mode: the build that is published on Cloudflare Pages (`DOCS_PUBLIC=1`).
//
// Default builds are unchanged by this module. In public mode:
//   - the origin comes from SITE_URL (required, see siteOrigin);
//   - the development history (docs/journal/) is left out of the site (docs-source.ts);
//   - links to it are dropped and their text kept (docs-links.ts);
//   - the names in public-redactions.json are replaced in every output (redact).
import { readFileSync } from "node:fs";
import { fileURLToPath } from "node:url";

export const PUBLIC_MODE = process.env.DOCS_PUBLIC === "1";

/** The origin of a local build. Public builds use SITE_URL instead. */
export const LOCAL_ORIGIN = "http://localhost:4321";

/** The origin the site is built for (canonical links, sitemap, llms.txt). */
export function siteOrigin(env: NodeJS.ProcessEnv = process.env): string {
  if (!PUBLIC_MODE) return LOCAL_ORIGIN;
  const raw = env.SITE_URL?.trim();
  if (!raw) {
    throw new Error(
      "DOCS_PUBLIC=1 needs SITE_URL, the public origin of the site, for example SITE_URL=https://effectgo-docs.pages.dev. The public build fails without it.",
    );
  }
  let url: URL;
  try {
    url = new URL(raw);
  } catch {
    throw new Error(`SITE_URL "${raw}" is not a URL. Use the origin only, for example https://effectgo-docs.pages.dev`);
  }
  if (url.protocol !== "https:" && url.protocol !== "http:") {
    throw new Error(`SITE_URL "${raw}" must be an http or https URL`);
  }
  if (url.pathname !== "/" || url.search || url.hash) {
    throw new Error(`SITE_URL "${raw}" must be an origin with no path, query or fragment`);
  }
  return url.origin;
}

/** The redaction mapping: name -> replacement. The file is public-redactions.json. */
export const REDACTIONS: Readonly<Record<string, string>> = JSON.parse(
  readFileSync(fileURLToPath(new URL("../public-redactions.json", import.meta.url)), "utf8"),
);

const escapeRegExp = (s: string) => s.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");
const KEYS = Object.keys(REDACTIONS).filter((k) => !k.startsWith("_") && k.length > 0);
const BY_LOWER = new Map(KEYS.map((k) => [k.toLowerCase(), REDACTIONS[k]]));
// Longest key first, so "arcteryx-jp" wins over "arcteryx" at the same position.
const REDACTION_RE = KEYS.length
  ? new RegExp(
      [...KEYS].sort((a, b) => b.length - a.length).map(escapeRegExp).join("|"),
      "gi",
    )
  : null;

/** Replaces every redacted name in `text`. Identity outside public mode. */
export function redact(text: string): string {
  if (!PUBLIC_MODE || !REDACTION_RE) return text;
  return text.replace(REDACTION_RE, (match) => BY_LOWER.get(match.toLowerCase()) ?? match);
}

/** Applies redact() to every string inside a frontmatter value. Identity outside public mode. */
export function redactData<T>(value: T): T {
  if (!PUBLIC_MODE) return value;
  if (typeof value === "string") return redact(value) as T;
  if (Array.isArray(value)) return value.map((v) => redactData(v)) as T;
  if (value !== null && typeof value === "object" && Object.getPrototypeOf(value) === Object.prototype) {
    return Object.fromEntries(
      Object.entries(value).map(([k, v]) => [k, redactData(v)]),
    ) as T;
  }
  return value;
}
