import type { APIRoute } from "astro";
import { getIndexedEntries } from "@cloudflare/nimbus-docs";
import { llmsRoute } from "@cloudflare/nimbus-docs/agent-endpoints";

export const prerender = true;

const base = llmsRoute();

// Nimbus's root index lists the top-level pages and links to one llms.txt per
// section. This site wants the pages themselves in the root index, so every
// page is listed under its section, with its description, after Nimbus's lines.
const SECTIONS = ["guide", "reference", "design", "journal"] as const;

export const GET: APIRoute = async (ctx) => {
  const res = await base.GET(ctx);
  if (!res.ok) return res;
  const entries = await getIndexedEntries();
  const site = ctx.site ?? new URL("http://localhost:4321");
  const lines: string[] = [];
  for (const section of SECTIONS) {
    const pages = entries
      .filter((e) => e.url.split("/")[1] === section)
      .sort((a, b) => a.url.localeCompare(b.url));
    if (pages.length === 0) continue;
    lines.push("", `## ${section[0].toUpperCase()}${section.slice(1)}`, "");
    for (const page of pages) {
      const url = new URL(page.markdownUrl, site).href;
      lines.push(`- [${page.title}](${url})${page.description ? ` — ${page.description}` : ""}`);
    }
  }
  const body = (await res.text()).trimEnd() + "\n" + lines.join("\n") + "\n";
  return new Response(body, { headers: res.headers });
};
