import { screen } from '@testing-library/react'

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

export function highlightedWords(text: string): (string | null)[] {
  return Array.from(answerRow(text).querySelectorAll('mark')).map(mark => mark.textContent)
}
