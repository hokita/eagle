import type { DiscussionMessage } from '@/lib/api'

interface Props {
  messages: DiscussionMessage[]
  refinements?: string[]
}

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
