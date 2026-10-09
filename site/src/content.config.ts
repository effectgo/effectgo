import { defineCollection } from "astro:content";
import { glob } from "astro/loaders";
// `z` re-exported from `astro:content` is deprecated; import it from
// `astro/zod` (the pattern nimbus-docs' own schema helpers document).
import { z } from "astro/zod";
import { defineDocSchema, withNimbusMarkdown } from "@cloudflare/nimbus-docs/content";
import { docsLinkLoader, docsRoot } from "./markdown/docs-links.ts";
import { DOCS_DIR, DOCS_PATTERN } from "./docs-source.ts";

// The docs are the repo's docs/ directory, read in place (no copy step).

export const collections = {
  docs: defineCollection({
    // Same wrapping as docsCollection(), with the link rewrite added inside it.
    loader: withNimbusMarkdown(
      docsLinkLoader(
        glob({
          base: DOCS_DIR,
          pattern: DOCS_PATTERN,
        }),
        { docsRoot },
      ),
    ),
    schema: defineDocSchema({
      fields: { audience: z.literal("human").optional() },
    }),
  }),
};
