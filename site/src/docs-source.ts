// Where the docs come from. Shared by the content collection (src/content.config.ts)
// and the lint script (scripts/lint-docs.mjs), so both see the same files.
//
// The pattern is an allowlist of the docs layout. docs/editor/ and anything
// else outside it is not part of the site until it is added here.
import { PUBLIC_MODE } from "./public-mode.ts";

/** Glob base, relative to the project root (site/). */
export const DOCS_DIR = "../docs";

/** Every page the site can build. Default builds use this. */
export const DOCS_PATTERN_ALL = "{index.md,{guide,reference,design,journal}/**/*.md}";

/** Public mode: the development history (journal/) is left out. */
export const DOCS_PATTERN_PUBLIC = "{index.md,{guide,reference,design}/**/*.md}";

/** Files under DOCS_DIR that form the site in this build. */
export const DOCS_PATTERN = PUBLIC_MODE ? DOCS_PATTERN_PUBLIC : DOCS_PATTERN_ALL;
