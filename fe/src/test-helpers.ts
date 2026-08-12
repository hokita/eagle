import { screen } from '@testing-library/react'

// The review panel renders an answer word by word so it can highlight the words
// that differ from the other answer, which splits the sentence across elements
// and hides it from a plain getByText. These helpers match on a whole row's
// text instead. The selector keeps the surrounding wrappers from matching too:
// answers always render as a p (the answer rows) or an li (past attempts).
const ROW_SELECTOR = 'p, li'

function matches(text: string) {
  return (_: string, node: Element | null) =>
    node?.textContent?.replace(/\s+/g, ' ').trim() === text
}

export function answerRow(text: string): HTMLElement {
  return screen.getByText(matches(text), { selector: ROW_SELECTOR })
}

export function answerRows(text: string): HTMLElement[] {
  return screen.queryAllByText(matches(text), { selector: ROW_SELECTOR })
}

// The words a row marks as differing from the answer it is compared against.
export function highlightedWords(text: string): (string | null)[] {
  return Array.from(answerRow(text).querySelectorAll('mark')).map(mark => mark.textContent)
}
