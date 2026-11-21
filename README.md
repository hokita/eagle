# Eagle 🦅

An English learning application that helps users improve their English skills by translating Japanese sentences. Built with Go backend and Next.js frontend.

## Overview

Eagle provides a platform for English language practice where users are presented with random Japanese sentences and can input their English translations. The app tracks answer history and provides feedback on translation accuracy.

## Features

- Random Japanese sentence display
- English translation input and validation
- Answer history tracking
- Responsive web interface
- Docker containerized deployment

## Muse token (one-time setup)

Muse, the owner's agent, practices with Eagle through the [MCP server](#mcp-muse). It authenticates with a bearer token kept in Secret Manager. The REST API still accepts only the signed-in app.

**One-time setup** (owner, once; the token is never printed)

```sh
# 1. Generate a token and store it in Secret Manager without showing it
openssl rand -hex 32 | tr -d '\n' | gcloud secrets create eagle-mcp-token \
  --project=eagle-473ac1 --replication-policy=automatic --data-file=-

# 2. Let the Cloud Run runtime service account read it
gcloud secrets add-iam-policy-binding eagle-mcp-token --project=eagle-473ac1 \
  --member=serviceAccount:eagle-api-runtime@eagle-473ac1.iam.gserviceaccount.com \
  --role=roles/secretmanager.secretAccessor

# 3. Look up the owner's Firebase uid (gcloud must be signed in as the owner's allowlisted account)
T=$(gcloud auth print-access-token)
UID_=$(curl -s -X POST -H "Authorization: Bearer $T" -H "x-goog-user-project: eagle-473ac1" \
  -H "Content-Type: application/json" \
  "https://identitytoolkit.googleapis.com/v1/projects/eagle-473ac1/accounts:lookup" \
  -d "{\"email\":[\"$(gcloud config get account)\"]}" \
  | python3 -I -c 'import json,sys; print(json.load(sys.stdin)["users"][0]["localId"])')

# 4. Wire both into deploys
gh variable set EAGLE_MCP_OWNER_UID -R hokita/eagle --body "$UID_"
gh variable set EAGLE_MCP_TOKEN_SECRET -R hokita/eagle --body eagle-mcp-token
unset T UID_
```

Until both variables are set, deploys run without the token and `/mcp` stays off.

**Deploy and check.** Merge to `main` (or run the Deploy API workflow) so the service redeploys, then:

```sh
URL=https://eagle-api-s64rarpnaa-an.a.run.app

# no token: expect 401
curl -s -o /dev/null -w '%{http_code}\n' -X POST $URL/mcp -H 'Content-Type: application/json' -d '{}'

# with the token: expect 200
TOKEN=$(gcloud secrets versions access latest --secret=eagle-mcp-token --project=eagle-473ac1)
curl -s -o /dev/null -w '%{http_code}\n' -X POST $URL/mcp \
  -H "Authorization: Bearer $TOKEN" -H "Content-Type: application/json" \
  -H "Accept: application/json, text/event-stream" \
  -d '{"jsonrpc": "2.0", "id": 1, "method": "initialize", "params": {"protocolVersion": "2025-06-18", "capabilities": {}, "clientInfo": {"name": "curl", "version": "0"}}}'
unset TOKEN
```

**Rotation.** Add a new secret version (`gcloud secrets versions add eagle-mcp-token --data-file=-`), redeploy, and update the token in the Muse connector.

## MCP (Muse)

The API serves an MCP server (streamable HTTP, stateless) at:

```
https://eagle-api-s64rarpnaa-an.a.run.app/mcp
Authorization: Bearer <token>
```

It uses the Muse token above and accepts nothing else (no Firebase sign-in); a missing or wrong token gets `401`. It acts as the owner's account.

| Tool                    | Arguments                                                  | Notes                                                                                                    |
| ----------------------- | ---------------------------------------------------------- | -------------------------------------------------------------------------------------------------------- |
| `get_practice_sentence` | `levels` (optional, each 1-5)                              | Same pick as the app (skips mastered and reported sentences); the English answer is not included          |
| `check_answer`          | `sentence_id`, `answer` (1-2000 bytes)                     | Grades and saves to history like the app; returns the correct answer and up to 5 previous wrong answers   |
| `list_mistakes`         | `limit` (1-50, default 10), `offset` (default 0)           | Past wrong answers, most recently missed first, with up to 3 latest wrong attempts each; read-only        |
| `report_sentence`       | `sentence_id`                                              | Marks a sentence as broken so it is no longer given; no undo, marked destructive so the client asks first |

No answer text from the REST API is exposed beyond what `check_answer` returns after an answer is submitted.

**Rate limit:** MCP tool calls are limited to 120 an hour. Protocol requests (`initialize`, `tools/list`) aren't counted. Over the limit, a tool call returns a tool error `Rate limit reached: try again in <N> seconds` (HTTP status stays 200).

**Muse setup** (after a deploy): add a custom connector with the URL above and the token as an `Authorization: Bearer <token>` header (read it with `gcloud secrets versions access latest --secret=eagle-mcp-token --project=eagle-473ac1`), then try each tool: get a sentence, answer it (it shows in the app's history, and on the Mistakes page if wrong), list mistakes, and report a throwaway sentence only if you mean it. If the connector can't send a custom `Authorization` header, it can't connect; OAuth isn't supported.

## Environment variables (API)

| Variable          | Description                                                                          |
| ----------------- | ------------------------------------------------------------------------------------ |
| `EAGLE_MCP_TOKEN` | Optional. Token Muse uses for `/mcp` (at least 43 chars); with `MCP_OWNER_UID` unset or this unset, `/mcp` is off |
| `MCP_OWNER_UID`   | Optional. Firebase uid of the owner account the MCP tools act as                      |
