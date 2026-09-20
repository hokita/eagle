# Free-Text Questions About an Explanation — Design Spec

## Problem

A learner who answers incorrectly can press **Explain** and read one
explanation of what happened. That explanation is a single shot: if it does
not land — an unfamiliar term, a rule the learner wants applied to a
different wording, "so was my answer actually acceptable?" — there is
nowhere to put the question. The learner has to leave the app to ask it.

## Goal

Let the learner ask their own free-text questions about the explanation they
were just shown, and read the answers in place, without losing the sentence,
their answer, or the explanation the question is about.

## Non-goals

- No persistence. Like the explanation itself, a thread lives only as long
  as the learner stays on that sentence — nothing is written to Firestore
  and nothing is shown again on a later visit.
- No questions without an explanation. The thread is about the explanation,
  so the box appears with it and not before.
- No streaming. Each answer is a short request/response, the same shape the
  explain endpoint already has.
- No general-purpose chat. The prompt keeps the model on this sentence and
  this explanation, and says what to do with an off-topic question.

## Flow

1. The learner answers incorrectly and presses **Explain** as today.
2. Under the explanation sits a question box. The learner types a question
   and presses **Ask** (or Ctrl+Enter).
3. The frontend posts the sentence id, the learner's translation, the
   explanation language, the explanation itself, the thread so far, and the
   new question to `POST /api/answer/followup`.
4. The backend loads the Japanese sentence and the reference answer from
   Firestore by sentence id, builds the prompt, and makes one non-streaming
   Gemini call.
5. The answer is appended to the thread, above the box, which is cleared for
   the next question.

## Backend

`Explainer` gains a second method, so both halves of the explain path share
one Gemini client and one model:

```go
AnswerFollowUp(ctx context.Context, in FollowUpInput) (string, error)
```

`FollowUpInput` carries the sentence, the reference answer, the learner's
translation, the explanation, the earlier turns, the new question, and the
language. `buildFollowUpPrompt` is a pure function, unit-tested without
network, like `buildExplainPrompt`.

**What the client may send.** The Japanese sentence and the reference answer
are loaded server-side by `sentence_id` and are rejected as unknown fields
if a client sends them — the same rule `explainAnswer` follows, for the same
reason: an authenticated caller must not be able to pass arbitrary text off
as the sentence being studied and have this app's API key process it. The
explanation and the earlier answers do come back from the client, because
nothing on this path is stored; they are bounded instead, in the same rune
units the frontend's `maxLength` counts:

| Field | Limit |
| --- | --- |
| `question`, and each history question | 500 runes |
| `explanation`, and each history answer | 4,000 runes |
| `user_answer` | `maxUserAnswerLength` (2,000) |
| whole body | 192 KiB |

Only the five most recent turns of a thread reach the model. A longer thread
is truncated rather than rejected — a learner who keeps asking is using the
feature — so the oldest questions fall out of the prompt while the thread
they read stays whole. The frontend sends the same five, so a long thread
never grows the request with it.

**Prompt guardrails.** The learner's question is free text, so the prompt
says what it is: a question to answer, never an instruction to follow. It
also tells the model what to do with a question that is not about English or
this sentence (say so briefly and invite one that is), and what to do when
the question is unclear (ask one short clarifying question). Answers are
capped at `maxFollowUpOutputTokens` and an empty one is a 500, not a blank
turn in the thread.

## Frontend

`ReviewPanel` renders the thread and owns the draft question — it is nobody
else's business until it is asked — clearing it only when an answer arrives,
so a failed request leaves the question in the box to retry. Answers render
through the same Markdown component as the explanation, with links and
images stripped.

`Translator` owns the thread itself, and drops it whenever the explanation
it belongs to goes away: a new sentence, or a re-fetch in the other
language. A follow-up answer still in flight when that happens is discarded
rather than landing under an explanation it was not asked about — the same
stale-response rule the explain request already follows.
