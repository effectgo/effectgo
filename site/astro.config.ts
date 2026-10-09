import { defineConfig } from "astro/config";
import tailwindcss from "@tailwindcss/vite";
import nimbus, {
  defineConfig as defineNimbusConfig,
} from "@cloudflare/nimbus-docs";
import { tableScroll } from "@cloudflare/nimbus-docs/markdown";
import { docsLinks } from "./src/markdown/docs-links.ts";

const nimbusConfig = defineNimbusConfig({
  // Local build only: this is the origin the canonical and llms.txt links are
  // written for. Replace it with the real origin before any deployment.
  site: "http://localhost:4321",
  title: "effectgo",
  description: "Documentation for effectgo, which infers Effect-style signatures for Go.",
  locale: "en",
  github: null,
  socialImageAlt: "effectgo documentation preview",
  sidebar: {
    items: [
      { label: "Guide", autogenerate: { directory: "guide" } },
      { label: "Reference", autogenerate: { directory: "reference" } },
      { label: "Design", autogenerate: { directory: "design" } },
      // Last, and collapsed by default: journal entries are history, not the
      // first thing a reader needs.
      { label: "Journal", autogenerate: { directory: "journal" }, collapsed: true },
    ],
  },
});

export default defineConfig({
  // nimbus:adapter
  output: "static",
  // Tailwind v4 via its Vite plugin (the integration Astro recommends for
  // Tailwind v4 — replaces the PostCSS plugin, which doesn't build under
  // Astro 7's Vite 8 bundler).
  vite: {
    plugins: [tailwindcss()],
  },
  // Hover-prefetch link targets so full-page navigations feel instant without
  // a client-side router.
  prefetch: {
    prefetchAll: true,
    defaultStrategy: "hover",
  },
  integrations: [
    nimbus(nimbusConfig, {
      rules: {
        "nimbus/frontmatter-shape": "error",
        // Docs links are relative `.md` links, which the rule reports by
        // default. allowRelative skips them; docs-links.ts checks them at build.
        "nimbus/internal-link": ["error", { allowRelative: true }],
      },
      markdown: {
        hastPlugins: [docsLinks(), tableScroll()],
      },
    }),
  ],
});
