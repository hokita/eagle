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

// visibleRefinements lines `refinements` up with `messages`: the entry at
// index i is the refinement shown under messages[i], or undefined when there
// is none to show. A refinement identical to its message is dropped — the
// coach returns a turn unchanged when it already sounded natural, and
// repeating it would read as a correction that changed nothing. Exported so
// a caller can tell whether anything will show before it introduces it.
export function visibleRefinements(
  messages: DiscussionMessage[],
  refinements: string[] | undefined
): (string | undefined)[] {
  let userIndex = 0
  return messages.map(message => {
    if (message.role !== 'user') return undefined
    const refined = refinements?.[userIndex++]
    if (refined === undefined || refined.trim() === message.text.trim()) return undefined
    return refined
  })
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
export default function Transcript({ messages, refinements }: Props) {
  const refined = visibleRefinements(messages, refinements)
  return (
    <div className="space-y-2">
      {messages.map((message, i) => (
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
          {refined[i] !== undefined && (
            <div className="mt-1">
              <span className="inline-block rounded-lg border border-indigo-200 bg-indigo-50 px-3 py-2 text-left text-sm text-indigo-900">
                <span className="block text-xs font-semibold uppercase tracking-wide text-indigo-500">
                  More natural
                </span>
                {refined[i]}
              </span>
            </div>
          )}
        </div>
      ))}
    </div>
  )
}
