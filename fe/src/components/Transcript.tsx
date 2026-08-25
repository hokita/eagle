import type { DiscussionMessage } from '@/lib/api'

interface Props {
  messages: DiscussionMessage[]
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
export default function Transcript({ messages }: Props) {
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
        </div>
      ))}
    </div>
  )
}
