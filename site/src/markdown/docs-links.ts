// Rewrites relative `.md` links between docs pages to the routes they build to.
//
// The same files are read by GitHub, where `../reference/directives.md` is the
// link target. On the site a page's URL has one more path segment than its
// source file (`guide/getting-started.md` -> `/guide/getting-started/`), so a
// relative link would resolve to the wrong page. Nimbus does not rewrite these
// links.
//
// Two places need the rewrite, and both use the same link decision:
//   - HTML: a Sätteri hast plugin (`docsLinks`). Astro renders the body from
//     the raw source, so the rewrite happens on the HTML, using the document's
//     fileURL. Sätteri does not run remark plugins, so this has to be hast.
//   - Stored entries: `docsLinkLoader` rewrites each `.md` body before Nimbus
//     stores it. The per-page Markdown and llms-full.txt come from that body.
//
// Public mode (DOCS_PUBLIC=1, see public-mode.ts) adds two things to both places:
//   - a link to a page that is left out of the site (docs/journal/) is not
//     a broken link: the link text stays and the link is dropped;
//   - every name in public-redactions.json is replaced, in the HTML (text and
//     attributes), in the stored bodies, and in the frontmatter values.
// Links are written as root-relative routes. Nimbus adds the site base to
// those when it stores the body. Files on disk are not changed.
import { existsSync } from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";
import type { Loader, LoaderContext } from "astro/loaders";
import type { NimbusIntegrationOptions } from "@cloudflare/nimbus-docs";
import { markdownToMdast } from "satteri";
import { DOCS_PATTERN, DOCS_PATTERN_ALL } from "../docs-source.ts";
import { PUBLIC_MODE, redact, redactData } from "../public-mode.ts";

/** The repo's docs/ directory, resolved from this file (site/src/markdown/). */
export const docsRoot = fileURLToPath(new URL("../../../docs/", import.meta.url));

type HastPlugin = NonNullable<NonNullable<NimbusIntegrationOptions["markdown"]>["hastPlugins"]>[number];
type MdNode = {
  type: string;
  url?: string;
  label?: string;
  identifier?: string;
  children?: MdNode[];
  position?: { start: { offset: number }; end: { offset: number } };
};
type Edit = { start: number; end: number; text: string };
/** What a relative docs link becomes. */
type LinkTarget = { kind: "route"; href: string } | { kind: "unlink" };

const SCHEME_OR_ROOT = /^(?:[a-z][a-z0-9+.-]*:|\/|#)/i;

/**
 * The decision for a relative `.md` destination written in `sourcePath`, or
 * undefined to leave it alone. A missing file, or one outside the site, throws.
 */
function linkTarget(dest: string, sourcePath: string, root: string): LinkTarget | undefined {
  if (SCHEME_OR_ROOT.test(dest)) return undefined;
  const cut = dest.search(/[?#]/);
  const file = cut === -1 ? dest : dest.slice(0, cut);
  const suffix = cut === -1 ? "" : dest.slice(cut);
  if (!file.endsWith(".md")) return undefined;
  const target = path.resolve(path.dirname(sourcePath), file);
  if (!target.startsWith(root + path.sep)) return undefined;
  // A docs link that names no file of the site fails the build. Lint skips
  // relative links (see astro.config.ts), so this is the check that catches them.
  const relFile = path.relative(root, target).split(path.sep).join("/");
  const broken = () =>
    new Error(
      `broken docs link "${dest}" in ${sourcePath}: ${relFile} is not a page of the site (DOCS_PATTERN in src/docs-source.ts)`,
    );
  if (!existsSync(target)) {
    throw new Error(`broken docs link "${dest}" in ${sourcePath}: no file at ${relFile}`);
  }
  if (path.matchesGlob(relFile, DOCS_PATTERN)) {
    const rel = relFile.replace(/\.md$/, "");
    let route: string;
    if (rel === "index") route = "/";
    else if (rel.endsWith("/index")) route = `/${rel.slice(0, -"index".length)}`;
    else route = `/${rel}/`;
    return { kind: "route", href: route + suffix };
  }
  // Public mode: a page left out of the site is not a broken link. Its link
  // text stays and the link is dropped.
  if (PUBLIC_MODE && path.matchesGlob(relFile, DOCS_PATTERN_ALL)) return { kind: "unlink" };
  throw broken();
}

/** Redacts the string values of an element's properties. Returns undefined when nothing changed. */
function redactProperties(props: Record<string, unknown>): Record<string, unknown> | undefined {
  let changed = false;
  const out: Record<string, unknown> = {};
  for (const [key, value] of Object.entries(props)) {
    if (typeof value === "string") {
      const next = redact(value);
      changed ||= next !== value;
      out[key] = next;
    } else if (Array.isArray(value) && value.every((v) => typeof v === "string")) {
      const next = value.map((v) => redact(v as string));
      changed ||= next.some((v, i) => v !== value[i]);
      out[key] = next;
    } else {
      out[key] = value;
    }
  }
  return changed ? out : undefined;
}

/** hast plugin for the HTML of each docs page. Needs the document's fileURL. */
export function docsLinks(options: { docsRoot: string } = { docsRoot }): HastPlugin {
  const root = path.resolve(options.docsRoot);
  return ((ctx: { fileURL?: URL }) => {
    if (!ctx.fileURL) return null;
    const sourcePath = fileURLToPath(ctx.fileURL);
    // Public mode visits every element (attributes are redacted too); a default
    // build visits only links, as before.
    const element = {
      filter: PUBLIC_MODE ? [] : ["a"],
      visit(node: { tagName?: string; properties?: Record<string, unknown> } & Record<string, unknown>) {
        let props = node.properties ?? {};
        if (node.tagName === "a" && typeof props.href === "string") {
          const target = linkTarget(props.href, sourcePath, root);
          if (target?.kind === "unlink") return { ...node, tagName: "span", properties: {} };
          if (target?.kind === "route") props = { ...props, href: target.href };
        }
        const redacted = PUBLIC_MODE ? redactProperties(props) : undefined;
        if (redacted) return { ...node, properties: redacted };
        if (props !== node.properties) return { ...node, properties: props };
        return undefined;
      },
    };
    const text = PUBLIC_MODE
      ? (node: { value: string } & Record<string, unknown>) => {
          const value = redact(node.value);
          return value === node.value ? undefined : { ...node, value };
        }
      : undefined;
    return { name: "effectgo:docs-links", element, ...(text ? { text } : {}) };
  }) as unknown as HastPlugin;
}

/**
 * Rewrites the relative `.md` links in a Markdown body (and, in public mode,
 * redacts the names). `sourcePath` is the file the body came from.
 */
export function rewriteDocLinks(source: string, sourcePath: string, root: string = docsRoot): string {
  // Parser offsets count code points, so work on code-point arrays.
  const cps = [...source];
  const tree = markdownToMdast(source) as unknown as MdNode;
  const edits: Edit[] = [];
  const isSpace = (c: string | undefined) => c === " " || c === "\t" || c === "\n";
  const normalize = (label: string) => label.trim().replace(/\s+/g, " ").toLowerCase();

  // Definitions whose target is dropped in public mode. Their references are dropped too.
  const droppedLabels = new Set<string>();
  const collect = (node: MdNode) => {
    if (node.type === "definition" && typeof node.url === "string" && node.position) {
      const dest = node.url;
      const target = linkTarget(dest, sourcePath, root);
      if (target?.kind === "unlink") droppedLabels.add(normalize(node.identifier ?? node.label ?? ""));
    }
    for (const child of node.children ?? []) collect(child);
  };
  collect(tree);

  // Replace [start, end) with the source text of the link's children.
  const unlink = (node: MdNode, start: number, stop: number) => {
    const first = node.children?.[0]?.position?.start.offset;
    const last = node.children?.at(-1)?.position?.end.offset;
    const text = first !== undefined && last !== undefined ? cps.slice(first, last).join("") : "";
    edits.push({ start, end: stop, text });
  };

  // Queue the destination at [start, end) when it is a `.md` link.
  const edit = (start: number, end: number, url: string) => {
    const dest = cps.slice(start, end).join("");
    if (dest !== url) return;
    const target = linkTarget(dest, sourcePath, root);
    if (target?.kind === "route") edits.push({ start, end, text: target.href });
  };

  const visit = (node: MdNode) => {
    const start = node.position?.start.offset;
    const stop = node.position?.end.offset;
    if (node.type === "link" && typeof node.url === "string" && start !== undefined && stop !== undefined) {
      // Inline link: the destination follows the first `](` after the link text.
      let i = node.children?.at(-1)?.position?.end.offset ?? start;
      while (i + 1 < stop && !(cps[i] === "]" && cps[i + 1] === "(")) i += 1;
      if (i + 1 < stop && cps[i] === "]" && cps[i + 1] === "(") {
        let s = i + 2;
        while (isSpace(cps[s])) s += 1;
        let e = s;
        while (e < stop && cps[e] !== ")" && !isSpace(cps[e])) e += 1;
        if (cps[s] !== "<") {
          const dest = cps.slice(s, e).join("");
          const target = dest === node.url ? linkTarget(dest, sourcePath, root) : undefined;
          if (target?.kind === "unlink") {
            unlink(node, start, stop);
            return;
          }
          edit(s, e, node.url);
        }
      }
    } else if (node.type === "linkReference" && start !== undefined && stop !== undefined && droppedLabels.has(normalize(node.identifier ?? node.label ?? ""))) {
      unlink(node, start, stop);
      return;
    } else if (node.type === "definition" && typeof node.url === "string" && start !== undefined) {
      // Reference definition: `[label]: destination`.
      let i = start;
      while (i + 1 < cps.length && !(cps[i] === "]" && cps[i + 1] === ":")) i += 1;
      let s = i + 2;
      while (isSpace(cps[s])) s += 1;
      let e = s;
      while (e < cps.length && !isSpace(cps[e])) e += 1;
      if (cps[s] !== "<") {
        const target = linkTarget(cps.slice(s, e).join(""), sourcePath, root);
        if (target?.kind === "unlink" && node.position) {
          // Drop the whole definition line, with its line break.
          let end = node.position.end.offset;
          if (cps[end] === "\n") end += 1;
          edits.push({ start, end, text: "" });
        } else {
          edit(s, e, node.url);
        }
      }
    }
    for (const child of node.children ?? []) visit(child);
  };
  visit(tree);

  let out = source;
  if (edits.length > 0) {
    for (const { start, end, text } of edits.sort((a, b) => b.start - a.start)) {
      cps.splice(start, end - start, ...text);
    }
    out = cps.join("");
  }
  return PUBLIC_MODE ? redact(out) : out;
}

/** Wraps a content loader so each `.md` entry is rewritten as it is stored (body, and frontmatter in public mode). */
export function docsLinkLoader(inner: Loader, options: { docsRoot: string } = { docsRoot }): Loader {
  const root = path.resolve(options.docsRoot);
  const load = inner.load.bind(inner);
  return {
    ...inner,
    name: inner.name,
    load: (context: LoaderContext) => {
      const projectRoot = fileURLToPath(context.config.root);
      const rewriteEntry = <T extends { body?: string; filePath?: string; data?: unknown }>(entry: T): T => {
        let next = entry;
        if (typeof entry.body === "string" && entry.filePath?.endsWith(".md")) {
          const body = rewriteDocLinks(entry.body, path.resolve(projectRoot, entry.filePath), root);
          if (body !== entry.body) next = { ...next, body };
        }
        if (PUBLIC_MODE && entry.data !== undefined) {
          // Titles and descriptions reach the page head, llms.txt and llms-full.txt.
          next = { ...next, data: redactData(entry.data) };
        }
        return next;
      };
      const store = context.store;
      const linkedStore = new Proxy(store, {
        get(target, prop) {
          if (prop === "set") return (entry: { body?: string; filePath?: string; data?: unknown }) => target.set(rewriteEntry(entry) as never);
          const value = Reflect.get(target, prop, target);
          return typeof value === "function" ? value.bind(target) : value;
        },
      });
      const linkedContext = new Proxy(context, {
        get(target, prop) {
          return prop === "store" ? linkedStore : Reflect.get(target, prop, target);
        },
      });
      return load(linkedContext);
    },
  };
}
