// Rewrites relative `.md` links between docs pages to the routes they build to.
//
// The same files are read by GitHub, where `../reference/directives.md` is the
// link target. On the site a page's URL has one more path segment than its
// source file (`guide/getting-started.md` -> `/guide/getting-started/`), so a
// relative link would resolve to the wrong page. Nimbus does not rewrite these
// links.
//
// Two places need the rewrite, and both use the same route mapping:
//   - HTML: a Sätteri hast plugin (`docsLinks`). Astro renders the body from
//     the raw source, so the rewrite happens on the HTML, using the document's
//     fileURL. Sätteri does not run remark plugins, so this has to be hast.
//   - Stored entries: `docsLinkLoader` rewrites each `.md` body before Nimbus
//     stores it. The per-page Markdown and llms-full.txt come from that body.
// Links are written as root-relative routes. Nimbus adds the site base to
// those when it stores the body. Files on disk are not changed.
import { existsSync } from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";
import type { Loader, LoaderContext } from "astro/loaders";
import type { NimbusIntegrationOptions } from "@cloudflare/nimbus-docs";
import { markdownToMdast } from "satteri";
import { DOCS_PATTERN } from "../docs-source.ts";

/** The repo's docs/ directory, resolved from this file (site/src/markdown/). */
export const docsRoot = fileURLToPath(new URL("../../../docs/", import.meta.url));

type HastPlugin = NonNullable<NonNullable<NimbusIntegrationOptions["markdown"]>["hastPlugins"]>[number];
type MdNode = {
  type: string;
  url?: string;
  children?: MdNode[];
  position?: { start: { offset: number }; end: { offset: number } };
};
type Edit = { start: number; end: number; text: string };

const SCHEME_OR_ROOT = /^(?:[a-z][a-z0-9+.-]*:|\/|#)/i;

/** The route for a relative `.md` destination written in `sourcePath`, or undefined to leave it alone. */
function routeFor(dest: string, sourcePath: string, root: string): string | undefined {
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
  if (!existsSync(target) || !path.matchesGlob(relFile, DOCS_PATTERN)) {
    throw new Error(
      `broken docs link "${dest}" in ${sourcePath}: ${relFile} is not a page of the site (DOCS_PATTERN in src/docs-source.ts)`,
    );
  }
  const rel = relFile.replace(/\.md$/, "");
  let route: string;
  if (rel === "index") route = "/";
  else if (rel.endsWith("/index")) route = `/${rel.slice(0, -"index".length)}`;
  else route = `/${rel}/`;
  return route + suffix;
}

/** hast plugin for the HTML of each docs page. Needs the document's fileURL. */
export function docsLinks(options: { docsRoot: string } = { docsRoot }): HastPlugin {
  const root = path.resolve(options.docsRoot);
  return ((ctx: { fileURL?: URL }) => {
    if (!ctx.fileURL) return null;
    const sourcePath = fileURLToPath(ctx.fileURL);
    return {
      name: "effectgo:docs-links",
      element: {
        filter: ["a"],
        visit(node: { properties?: Record<string, unknown> }) {
          const href = node.properties?.href;
          if (typeof href !== "string") return;
          const next = routeFor(href, sourcePath, root);
          if (next === undefined) return;
          return { ...node, properties: { ...node.properties, href: next } };
        },
      },
    };
  }) as unknown as HastPlugin;
}

/** Rewrites the relative `.md` links in a Markdown body. `sourcePath` is the file the body came from. */
export function rewriteDocLinks(source: string, sourcePath: string, root: string = docsRoot): string {
  // Parser offsets count code points, so work on code-point arrays.
  const cps = [...source];
  const tree = markdownToMdast(source) as unknown as MdNode;
  const edits: Edit[] = [];

  // Queue the destination at [start, end) when it is a `.md` link.
  const edit = (start: number, end: number, url: string) => {
    const dest = cps.slice(start, end).join("");
    if (dest !== url) return;
    const next = routeFor(dest, sourcePath, root);
    if (next !== undefined) edits.push({ start, end, text: next });
  };
  const isSpace = (c: string | undefined) => c === " " || c === "\t" || c === "\n";

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
        if (cps[s] !== "<") edit(s, e, node.url);
      }
    } else if (node.type === "definition" && typeof node.url === "string" && start !== undefined) {
      // Reference definition: `[label]: destination`.
      let i = start;
      while (i + 1 < cps.length && !(cps[i] === "]" && cps[i + 1] === ":")) i += 1;
      let s = i + 2;
      while (isSpace(cps[s])) s += 1;
      let e = s;
      while (e < cps.length && !isSpace(cps[e])) e += 1;
      if (cps[s] !== "<") edit(s, e, node.url);
    }
    for (const child of node.children ?? []) visit(child);
  };
  visit(tree);

  if (edits.length === 0) return source;
  for (const { start, end, text } of edits.sort((a, b) => b.start - a.start)) {
    cps.splice(start, end - start, ...text);
  }
  return cps.join("");
}

/** Wraps a content loader so each `.md` entry body is rewritten as it is stored. */
export function docsLinkLoader(inner: Loader, options: { docsRoot: string } = { docsRoot }): Loader {
  const root = path.resolve(options.docsRoot);
  const load = inner.load.bind(inner);
  return {
    ...inner,
    name: inner.name,
    load: (context: LoaderContext) => {
      const projectRoot = fileURLToPath(context.config.root);
      const rewriteEntry = <T extends { body?: string; filePath?: string }>(entry: T): T => {
        if (typeof entry.body !== "string" || !entry.filePath?.endsWith(".md")) return entry;
        const body = rewriteDocLinks(entry.body, path.resolve(projectRoot, entry.filePath), root);
        return body === entry.body ? entry : { ...entry, body };
      };
      const store = context.store;
      const linkedStore = new Proxy(store, {
        get(target, prop) {
          if (prop === "set") return (entry: { body?: string; filePath?: string }) => target.set(rewriteEntry(entry) as never);
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
