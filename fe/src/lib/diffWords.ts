export interface DiffWord {
  text: string
  changed: boolean
}

export interface WordDiff {
  user: DiffWord[]
  correct: DiffWord[]
}

// Answers are single sentences, but the API accepts up to 2000 characters. The
// diff below is quadratic in the word count, so give up on highlighting rather
// than grind through a pathological input: an answer that long is unreadable as
// a comparison anyway.
const MAX_WORDS = 300

// The backend grades with EqualFold over whitespace-collapsed text, so the diff
// has to agree with it: whitespace and letter case never count as a difference,
// or a word would be marked wrong in an answer that was accepted as right.
function tokenize(sentence: string): string[] {
  return sentence.split(/\s+/).filter(Boolean)
}

function same(a: string, b: string): boolean {
  return a.toLowerCase() === b.toLowerCase()
}

function keep(words: string[]): DiffWord[] {
  return words.map(text => ({ text, changed: false }))
}

// diffWords aligns the two answers word by word and marks the words that do not
// line up, so the reader can spot what to change instead of re-reading both
// sentences. Alignment is a longest-common-subsequence walk: words the two
// answers share stay unmarked in both, and everything else is marked as an
// extra word on the user's side or a missing word on the correct side.
export function diffWords(userAnswer: string, correctAnswer: string): WordDiff {
  const user = tokenize(userAnswer)
  const correct = tokenize(correctAnswer)

  if (user.length > MAX_WORDS || correct.length > MAX_WORDS) {
    return { user: keep(user), correct: keep(correct) }
  }

  // lcs[i][j] is the length of the longest common subsequence of user[i:] and
  // correct[j:], filled from the end so the walk below can read it forwards.
  const lcs: number[][] = Array.from({ length: user.length + 1 }, () =>
    new Array<number>(correct.length + 1).fill(0),
  )
  for (let i = user.length - 1; i >= 0; i--) {
    for (let j = correct.length - 1; j >= 0; j--) {
      lcs[i][j] = same(user[i], correct[j])
        ? lcs[i + 1][j + 1] + 1
        : Math.max(lcs[i + 1][j], lcs[i][j + 1])
    }
  }

  const userDiff: DiffWord[] = []
  const correctDiff: DiffWord[] = []
  let i = 0
  let j = 0
  while (i < user.length && j < correct.length) {
    if (same(user[i], correct[j])) {
      userDiff.push({ text: user[i++], changed: false })
      correctDiff.push({ text: correct[j++], changed: false })
    } else if (lcs[i + 1][j] >= lcs[i][j + 1]) {
      userDiff.push({ text: user[i++], changed: true })
    } else {
      correctDiff.push({ text: correct[j++], changed: true })
    }
  }
  while (i < user.length) userDiff.push({ text: user[i++], changed: true })
  while (j < correct.length) correctDiff.push({ text: correct[j++], changed: true })

  return { user: userDiff, correct: correctDiff }
}
