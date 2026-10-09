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

## Where the content comes from

- `src/content.config.ts` reads `../docs/` in place. There is no copy step for the site.
- Only the pages in `DOCS_PATTERN` (`src/docs-source.ts`) are part of the site: `index.md` and the `guide/`, `reference/`, `design/` and `journal/` directories. Anything else under `docs/` (such as `docs/editor/`) is not built until it is added to that pattern.
- The sidebar groups are set in `astro.config.ts`: Guide, Reference, Design, then Journal, collapsed.

## Relative links

Docs pages link to each other with relative `.md` links, such as `../reference/directives.md`. These work on GitHub. On the site they are rewritten to the page routes (`/reference/directives/`):

- `src/markdown/docs-links.ts` rewrites them in the HTML (a Sätteri hast plugin) and in the stored page bodies, which feed the per-page Markdown and `llms-full.txt`.
- A link that names a file which does not exist fails the build.
- Lint skips relative links (`allowRelative` in `astro.config.ts`), because the rewrite checks them instead.

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
