import { defineConfig } from "cf/config";

export default defineConfig({
  worker: {
    "name": "effectgo-docs",
    "compatibilityDate": "2026-09-25",
    "observability": {
      "enabled": true
    },
    "assets": {
      "notFoundHandling": "404-page"
    }
  }
});
