// Client for the two Go endpoints, through the generated Api (api.gen.ts, made by
// `effectgo ts`). Typed errors are handled with catchTag. Run: npx tsx client.ts
// Base URL: BASE_URL env (default http://127.0.0.1:8080).

import { Effect, Schema } from "effect"
import { HttpApiClient } from "effect/http-api"
import { NodeHttpClient } from "@effect/platform-node"
import { Api, ContactInput, ContactNotFound, ValidationError } from "./api.gen.ts"

const baseUrl = process.env.BASE_URL ?? "http://127.0.0.1:8080"

const program = Effect.gen(function* () {
  const client = yield* HttpApiClient.make(Api, { baseUrl })

  const get = (id: number) =>
    client.store.getContact({ params: { id } }).pipe(
      Effect.map((c) => `200 ${JSON.stringify(c)}`),
      Effect.catchTag("ContactNotFound", (e) =>
        Effect.succeed(`404 caught catchTag("ContactNotFound"): instanceof ContactNotFound=${e instanceof ContactNotFound} _tag=${e._tag} detail=${JSON.stringify(e.detail)}`),
      ),
    )

  const update = (payload: typeof ContactInput.Type) =>
    client.store.updateContact({ payload }).pipe(
      Effect.map((c) => `200 ${JSON.stringify(c)}`),
      Effect.catchTag("ContactNotFound", (e) =>
        Effect.succeed(`404 caught catchTag("ContactNotFound"): instanceof ContactNotFound=${e instanceof ContactNotFound} _tag=${e._tag} detail=${JSON.stringify(e.detail)}`),
      ),
      Effect.catchTag("ValidationError", (e) =>
        Effect.succeed(`400 caught catchTag("ValidationError"): instanceof ValidationError=${e instanceof ValidationError} _tag=${e._tag} detail=${JSON.stringify(e.detail)}`),
      ),
    )

  const results = [
    ["GET /api/contacts/1", yield* get(1)],
    ["GET /api/contacts/999", yield* get(999)],
    ["POST /api/contact id=1", yield* update({ id: 1, name: "Ada Lovelace", email: "ada@example.org" })],
    ["POST /api/contact name=\"\"", yield* update({ id: 1, name: "", email: "ada@example.org" })],
    ["POST /api/contact id=999", yield* update({ id: 999, name: "Bob", email: "bob@example.com" })],
  ] as const
  for (const [label, line] of results) {
    console.log(`${label.padEnd(28)} -> ${line}`)
  }
})

Effect.runPromise(program.pipe(Effect.provide(NodeHttpClient.layerUndici))).catch((err) => {
  console.error("UNHANDLED (not a typed endpoint error):", err)
  process.exitCode = 1
})
