# effectgo documentation site

A [Nimbus](https://github.com/cloudflare/nimbus) site (Astro, static output) that renders the repository's `../docs/` directory for people and for agents. It is a local build only: nothing here deploys.

The Markdown in `../docs/` stays plain, so it reads the same on GitHub and on the site.

## Commands

Run these in this directory (`site/`), with pnpm:

| Command | What it does |
| --- | --- |
| `pnpm install` | Install the dependencies (lockfile: `pnpm-lock.yaml`) |
| `pnpm dev` | Dev server with hot reload |
| `pnpm build` | Static build into `dist/`, plus Pagefind search and the agent outputs |
| `pnpm preview` | Serve `dist/` locally |
| `pnpm typecheck` | `astro check` |
| `pnpm lint:docs` | Build, then run the Nimbus authoring linter over the docs (see below) |
| `pnpm build:public` | The public build (`DOCS_PUBLIC=1 pnpm build`), with `SITE_URL` required. See [Public build](#public-build) |
| `pnpm check:public` | Scans `dist/` for names that must not be published. Run it after `pnpm build:public` |

## Where the content comes from

- `src/content.config.ts` reads `../docs/` in place. There is no copy step for the site.
- Only the pages in `DOCS_PATTERN` (`src/docs-source.ts`) are part of the site: `index.md` and the `guide/`, `reference/`, `design/` and `journal/` directories. Anything else under `docs/` (such as `docs/editor/`) is not built until it is added to that pattern. The public build uses `DOCS_PATTERN_PUBLIC`, which leaves `journal/` out.
- The sidebar groups are set in `astro.config.ts`: Guide, Reference, Design, then Journal, collapsed. The public build has no Journal group.

## Relative links

Docs pages link to each other with relative `.md` links, such as `../reference/directives.md`. These work on GitHub. On the site they are rewritten to the page routes (`/reference/directives/`):

- `src/markdown/docs-links.ts` rewrites them in the HTML (a Sätteri hast plugin) and in the stored page bodies, which feed the per-page Markdown and `llms-full.txt`.
- A link that names a file which does not exist fails the build. A link to a journal page is a normal page link in the default build. In the public build, where the journal is left out, it keeps its text and drops the link (see [Public build](#public-build)).
- Lint skips relative links (`allowRelative` in `astro.config.ts`), because the rewrite checks them instead.

## Public build

The site has two builds. The default build (`pnpm build`) is for local use and includes everything in `../docs/`. The public build is the one published on Cloudflare Pages.

**`../docs/` itself is not redacted.** It stays the full repository documentation, including `docs/journal/`, which names private production services. Nothing in `docs/` changes for the public build. All filtering happens at site build time, and only when `DOCS_PUBLIC=1` is set. **The public mode output (`pnpm build:public`) is what gets deployed.** Never deploy the output of `pnpm build`.

To build and check the public site:

```sh
SITE_URL=https://effectgo-docs.pages.dev pnpm build:public
pnpm check:public
```

- `SITE_URL` is required. The build fails without it. It sets the origin for canonical links, the sitemap and `llms.txt`. Give the origin only (no path).
- The public build leaves `docs/journal/` out. Its pages, its `llms.txt` entries, its sitemap entries, its search index entries and its Markdown outputs are all absent, and the sidebar has no Journal group.
- A link from a published page to a journal page keeps its text and drops the link, in the HTML and in the stored Markdown (`llms-full.txt`, per-page `.md`). In the default build those links stay links.
- Names in `public-redactions.json` are replaced in every output: page bodies (HTML and Markdown), titles and descriptions (frontmatter), attributes such as `href` and `id`, `llms.txt`, `llms-full.txt` and the search index. The matching is case-insensitive and substring-based, and the longest name wins. To hide another name, add it to that file. The mapping is applied in `src/markdown/docs-links.ts` (the hast plugin and the loader wrapper) and in `src/public-mode.ts`.
- `scripts/check-public.mjs` scans every file in `dist/` for a denylist (the redacted names, `VectorSprint`, paths and hosts of the private systems, and `/journal/` in any URL). Compressed files are scanned in their decompressed form too, which covers the Pagefind fragments. It exits 1 and lists each hit with its file and context. The denylist includes every key of `public-redactions.json`. Its only exceptions are the editor guide's VS Code wording "user or workspace settings", "workspace scope" and "in the workspace. Add", which are listed by exact phrase in `ALLOWED_PHRASES`.

A passing check is necessary but not a full review. Read the public pages before deployment.

## Agent outputs

After `pnpm build`, `dist/` contains:

- `llms.txt`: an index of the pages, each with its description; per-section `llms.txt` files too.
- `llms-full.txt`: every page as Markdown in one file.
- A Markdown version of each page, at `<route>/index.md`, plus the authored source at `<route>/index.mdx`.

`src/pages/llms.txt.ts` extends Nimbus's root index so that every page is listed under its section.

## Lint rules

`pnpm lint:docs` copies the docs into a gitignored `.lint-docs/` mirror as `.mdx` files and runs `nimbus-docs lint` there, because the linter only reads `.mdx` under `src/content`. The rules are the ones enabled in `astro.config.ts`. Run `pnpm build` first, or let `lint:docs` do it.

## Versions

Tracking latest: `@cloudflare/create-nimbus-docs` 0.7.11 (scaffolder), `@cloudflare/nimbus-docs` 0.17.0, `astro` 7.3.1. The exact versions are in `pnpm-lock.yaml`.

## Deploy (public site)

The public site is served by Cloudflare (Pages on Workers, static assets only) at
https://effectgo-docs.faintghost.workers.dev, deployed with the `cf` CLI:

```sh
pnpm deploy:public            # build:public → check:public → copy dist → cf deploy
pnpm deploy:public --dry-run  # everything except the upload
```

`scripts/deploy-public.sh` builds in public mode (journal excluded, private
names redacted), refuses to continue if `check:public` finds a denylisted term,
copies the result into `deploy/public/`, and runs `cf deploy` from `deploy/`.
`deploy/` holds only the Cloudflare config (`cloudflare.config.ts`,
`wrangler.config.ts`) and its own `package.json`; its `public/`, `node_modules/`
and `.cloudflare/` are gitignored. Because `deploy/` contains no site source,
`cf deploy` cannot build or upload the unredacted default site. Never point a
deploy at a default build.

Override the origin with `SITE_URL=… pnpm deploy:public` if the Worker URL changes.
