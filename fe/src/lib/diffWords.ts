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

// The backend grades with EqualFold over text that has been whitespace-
// collapsed, folded to ASCII punctuation and stripped of the sentence-ending
// punctuation at its very end (normalizeAnswer in api/internal/app/handlers.go).
// The diff has to agree with it: whatever the grader ignores must never count as
// a difference here, or a word would be marked wrong in an answer that was
// accepted as right.
const ASCII_PUNCTUATION: Record<string, string> = {
  '‘': "'",
  '’': "'",
  ʼ: "'",
  '′': "'",
  '＇': "'",
  '“': '"',
  '”': '"',
  '″': '"',
  '＂': '"',
  '‐': '-',
  '‑': '-',
  '–': '-',
  '—': '-',
  '，': ',',
}
const TYPOGRAPHIC = new RegExp(`[${Object.keys(ASCII_PUNCTUATION).join('')}]`, 'g')

// The ASCII, fullwidth and Japanese sentence-ending marks the grader trims from
// the end of an answer.
const TERMINAL = '.!?…。．！？'
const TRAILING_TERMINAL = new RegExp(`[${TERMINAL}]+$`)
const ONLY_TERMINAL = new RegExp(`^[${TERMINAL}]+$`)

function tokenize(sentence: string): string[] {
  const words = sentence.split(/\s+/).filter(Boolean)
  // The grader trims the trailing punctuation together with the space in front
  // of it, so a detached final "." is not a word to compare either.
  while (words.length > 0 && ONLY_TERMINAL.test(words[words.length - 1])) words.pop()
  return words
}

// compareKeys renders each word as the grader sees it: typographic punctuation
// folded to ASCII and letter case dropped, plus — for the last word only — the
// sentence-ending punctuation the grader trims off the end of the answer.
// Punctuation inside the sentence still counts, exactly as it does when grading.
function compareKeys(words: string[]): string[] {
  return words.map((word, index) => {
    const folded = word.replace(TYPOGRAPHIC, char => ASCII_PUNCTUATION[char]).toLowerCase()
    return index === words.length - 1 ? folded.replace(TRAILING_TERMINAL, '') : folded
  })
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

  // The words are aligned on their grading keys but displayed as they were
  // written, so the reader sees their own answer back verbatim.
  const userKeys = compareKeys(user)
  const correctKeys = compareKeys(correct)
  const same = (i: number, j: number) => userKeys[i] === correctKeys[j]

  // lcs[i][j] is the length of the longest common subsequence of user[i:] and
  // correct[j:], filled from the end so the walk below can read it forwards.
  const lcs: number[][] = Array.from({ length: user.length + 1 }, () =>
    new Array<number>(correct.length + 1).fill(0),
  )
  for (let i = user.length - 1; i >= 0; i--) {
    for (let j = correct.length - 1; j >= 0; j--) {
      lcs[i][j] = same(i, j) ? lcs[i + 1][j + 1] + 1 : Math.max(lcs[i + 1][j], lcs[i][j + 1])
    }
  }

  const userDiff: DiffWord[] = []
  const correctDiff: DiffWord[] = []
  let i = 0
  let j = 0
  while (i < user.length && j < correct.length) {
    if (same(i, j)) {
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
