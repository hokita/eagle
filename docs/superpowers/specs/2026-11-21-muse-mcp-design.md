# Muse connection (MCP server) — design

Date: 2026-11-21. Status: design approved by the owner on 2026-11-21. Details marked **my call** were
settled while writing this doc, from the code and the owlet reference.

## Intent

The owner practices English while talking to their agent Muse. Muse asks eagle for a Japanese sentence,
the owner answers to Muse, and eagle grades the answer and saves it to the same history the app uses.
Muse coaches from eagle's mistake list itself. The app stays the app; Muse is a second way into the same
practice loop.

## Goals

- Exactly four MCP tools: get a practice sentence, check an answer, list mistakes, report a bad sentence.
- Same behaviour as the app's REST endpoints. The same random pick, the same grading
  (`normalizeAnswer` + case-fold), and the same Firestore writes, so a Muse answer looks like an app
  answer on the Mistakes page and in "Previous Incorrect Answers".
- Muse connects as a custom connector: URL `<eagle-api>/mcp`, header `Authorization: Bearer <token>`.

## Non-goals

- No explain, follow-up, mistake insight, discussion, or phrase tools.
- No OAuth; no change to the app's own Firebase sign-in.
- No multi-user MCP: one token, one owner.

## Anti-goals (would be failures even if it "works")

- Muse sees the reference English before the owner answers. `/api/sentence/random` returns `english`,
  so the MCP tool must not pass it through.
- Muse's answers land in an empty or wrong history (for example under a made-up uid), so the app never
  shows them.
- The MCP token opens any REST endpoint, or a Firebase ID token opens `/mcp`.
- A deploy breaks before the token exists.

## Constraints (found in code and GCP)

- History is keyed by **Firebase uid** (`users/{uid}/sentence_stats/{sentenceId}/histories`), not by
  email. `ALLOWED_EMAILS` (secret `ALLOWED_EMAIL`) holds **two** addresses, and Firestore has two
  `users/*` docs. "The allowed email" is therefore ambiguous: the MCP owner must be named explicitly.
- The Cloud Run service `eagle-api` runs in `asia-northeast1` as `eagle-api-runtime@…`, which has only
  `roles/datastore.user`. It is deployed by `.github/workflows/deploy-api.yml` on push to `main`.
  `hokita/eagle_deploy` (k8s manifests, one commit from 2025) is legacy and not in the deploy path.
- Go toolchain: `go.mod` says `go 1.25.0`, the Dockerfile builds with `golang:1.25.0-alpine`, and the
  workflows install Go 1.23, which auto-downloads 1.25.0. Go 1.25 is out of support (current: 1.27.4 /
  1.26.11), so production is built with a Go that no longer gets security fixes. This is fixed as Task 0
  of the plan.
- MCP SDK: `github.com/modelcontextprotocol/go-sdk` **v1.8.0** (latest stable, 2026-09-04; `go 1.25.0`).
  It has a stateless streamable HTTP handler, and a plain Go error returned from a tool handler becomes
  an `isError` tool result.

## Approaches

1. **`/mcp` inside the existing Go API** (chosen, same as owlet). It reuses the `Server`, the repo, and
   the deploy; only a token middleware and a tool layer are new.
2. **A separate MCP service** (rejected). It would need a second Cloud Run service, a second deploy, and
   a second auth and IAM setup, and it would either call the REST API (which only accepts Firebase ID
   tokens) or duplicate the repo code. That is more to run for no gain.
3. **MCP calls the REST API over HTTP with a service identity** (rejected). The REST API would have to
   learn a second auth scheme, which is the anti-goal above.

## Design

**Mounting.** `NewMux` gains an MCP config. `/mcp` is registered with no CORS (it is server-to-server)
and wrapped by `requireMCPToken`. The REST routes keep `requireAuth` unchanged.

**Token.** The value comes from the `EAGLE_MCP_TOKEN` env var, sourced from the Secret Manager secret
`eagle-mcp-token`. The matcher is ported from owlet's `apiTokenMatcher`: sha256 both sides,
`subtle.ConstantTimeCompare`, and off (nil) when unset or shorter than 43 chars. An off matcher means
every `/mcp` request gets `401`. A missing, wrong, or short token also gets `401` with body
`{"error":"Invalid token"}`.

**Owner (my call).** `MCP_OWNER_UID` is the Firebase uid of the owner's allowlisted account. It is set
from the repo variable `EAGLE_MCP_OWNER_UID`. `/mcp` is on only when both the token and the owner uid
are set. If either is missing, startup logs a warning and every `/mcp` request gets `401`. Rejected
alternatives:
- Resolving email → uid at runtime. With two allowed emails it would still need a "which email" setting,
  and it would add `roles/firebaseauth.viewer` and an Auth call to the runtime.
- Picking `ALLOWED_EMAILS[0]`. Order-dependent and silent.

The worker looks the uid up once with the Identity Toolkit API, using the gcloud account (which is the
owner's allowlisted account) so the email is never typed or printed.

**Rate limit.** Ported from owlet: a rolling window kept in memory per instance, guarded by a mutex.
Only tool calls take from it; `initialize` and `tools/list` don't count. Over the limit, the tool returns
`Rate limit reached: try again in N seconds`. The limit is **120 tool calls an hour (my call)**: one
sentence costs two calls (get + check), so this allows about 60 sentences an hour. That is well above
real practice and still bounds `RandomCandidate`, which reads the whole `sentences` collection per call.

**Transport (my call).** `mcp.NewStreamableHTTPHandler` with `Stateless: true` and
`JSONResponse: true`, so curl smoke checks and the e2e test read plain JSON. `MaxRequestBodyBytes` is
64 KiB, the SDK default for localhost protection stays on, and one `mcp.Server` is built per request,
like owlet.

**Shared logic (my call).** The REST handlers' core moves into `Server` methods that both REST and MCP
call:
- `randomSentence(ctx, uid, levels)`
- `checkAndRecord(ctx, uid, id, answer)`
- `mistakes(ctx, uid)`
- `report(ctx, id)`

Grading can then never drift between the two. REST responses stay byte-for-byte the same, except for
one fix: `Report` on a missing id now returns `ErrNotFound`. REST maps it to `404` instead of `500`,
and MCP returns a clear tool error.

**Tools.** The exact names, descriptions, and schemas are in the plan.

| Tool | In | Out | Notes |
|---|---|---|---|
| `get_practice_sentence` | `levels?` (ints 1–5) | `id`, `japanese`, `level`, `correct_count`, `incorrect_count` | **No `english`** (my call); read-only |
| `check_answer` | `sentence_id`, `answer` (1–2000 bytes, not blank) | `is_correct`, `correct_answer`, `your_answer`, `previous_wrong_answers` (latest 5) | Records like the app; blank answers are rejected (my call: an empty Muse call is a bug, not an attempt) |
| `list_mistakes` | `limit?` (1–50, default 10), `offset?` (≥0) | `total`, `has_more`, `mistakes[]` with latest 3 wrong answers each | Read-only; caps chosen so one result stays small (my call) |
| `report_sentence` | `sentence_id` | confirmation text | Hides the sentence from rotation, with no undo in the app, so `destructiveHint: true` and Muse asks first (my call) |

Errors are always MCP tool errors (`isError: true`, HTTP 200) with plain messages, for example
`Sentence 123 not found` or `answer must be 1–2000 bytes`. Internal failures are logged and returned
as `Internal error`, never as raw Firestore text.

**Deploy.** `deploy-api.yml` gets owlet's optional switch:
- `EAGLE_MCP_TOKEN=${vars.EAGLE_MCP_TOKEN_SECRET}:latest` is appended to `--set-secrets` only when that
  repo variable is set.
- `MCP_OWNER_UID` is appended to the env vars only when `vars.EAGLE_MCP_OWNER_UID` is set.

Deploys keep working before either exists.

## Left to the builder

- Go file layout under `api/internal/app` (suggested: `mcp_token.go`, `ratelimit.go`, `mcp.go`, each
  with tests).
- Exact log wording.
- Whether `e2eserver` reads the same env vars as `main.go` (it should).
