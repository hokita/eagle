import type { DiscussionMessage } from '@/lib/api'

interface Props {
  messages: DiscussionMessage[]
  // One entry per user message in `messages`, in order: that message the way
  // a native speaker would say it. Omitted during the live chat, where no
  // teaching happens; supplied by the summary and history, which show each
  // refinement directly under the message it refines so the learner can hold
  // the two side by side.
  refinements?: string[]
}

// The one read-only rendering of a discussion conversation, shared by the live
// chat, the post-session summary, and the history detail panel.
//
// It is the chat's own bubble layout deliberately: the learner reads the
// conversation this way while having it, so replaying it later in a different
// shape — flat "You: " paragraphs, as history and the summary each did — makes
// the same content read as something else, and runs the turns together into
// one block with nothing for the eye to break on.
//
// The opening question is not rendered here. It never enters the transcript:
// the conversation starts empty and only the coach's follow-ups are appended,
// so transcript[0] is the learner's first answer. Each caller supplies the
// question from where it already holds it — the chat and the summary above
// this list, history as its card heading.
//
// Every learner message that has a refinement gets something under it. The
// coach returns a turn unchanged when it already sounded natural; that turn
// is marked "Already natural" rather than left bare, because next to
// neighbours that each got a rewrite a bare message reads as one the coach
// skipped, not one it approved.
export default function Transcript({ messages, refinements }: Props) {
  let userIndex = 0
  return (
    <div className="space-y-2">
      {messages.map((message, i) => {
        const refined = message.role === 'user' ? refinements?.[userIndex++] : undefined
        const unchanged = refined !== undefined && refined.trim() === message.text.trim()
        return (
          <div key={i} className={message.role === 'user' ? 'text-right' : 'text-left'}>
            <span
              className={
                message.role === 'user'
                  ? 'inline-block rounded-lg bg-indigo-600 px-3 py-2 text-sm text-white'
                  : 'inline-block rounded-lg border border-border bg-muted px-3 py-2 text-sm text-foreground'
              }
            >
              {message.text}
            </span>
            {refined !== undefined && !unchanged && (
              <div className="mt-1">
                <span className="inline-block rounded-lg border border-indigo-200 bg-indigo-50 px-3 py-2 text-left text-sm text-indigo-900">
                  <span className="block text-xs font-semibold uppercase tracking-wide text-indigo-500">
                    More natural
                  </span>
                  {refined}
                </span>
              </div>
            )}
            {unchanged && (
              <div className="mt-1">
                <span className="inline-block rounded-md border border-emerald-200 bg-emerald-50 px-2 py-0.5 text-xs font-semibold uppercase tracking-wide text-emerald-700">
                  Already natural
                </span>
              </div>
            )}
          </div>
        )
      })}
    </div>
  )
}
