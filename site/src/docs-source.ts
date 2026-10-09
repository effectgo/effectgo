// Where the docs come from. Shared by the content collection (src/content.config.ts)
// and the lint script (scripts/lint-docs.mjs), so both see the same files.
//
// The pattern is an allowlist of the docs layout. docs/editor/ and anything
// else outside it is not part of the site until it is added here.

/** Glob base, relative to the project root (site/). */
export const DOCS_DIR = "../docs";

/** Files under DOCS_DIR that form the site. */
export const DOCS_PATTERN = "{index.md,{guide,reference,design,journal}/**/*.md}";
