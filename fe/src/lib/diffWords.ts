export interface DiffWord {
  text: string
  changed: boolean
}

export interface WordDiff {
  user: DiffWord[]
  correct: DiffWord[]
}

const MAX_WORDS = 300

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

const TERMINAL = '.!?…。．！？'
const TRAILING_TERMINAL = new RegExp(`[${TERMINAL}]+$`)
const ONLY_TERMINAL = new RegExp(`^[${TERMINAL}]+$`)

function tokenize(sentence: string): string[] {
  return sentence.split(/\s+/).filter(Boolean)
}

function trailingPunctuation(words: string[]): string[] {
  let start = words.length
  while (start > 0 && ONLY_TERMINAL.test(words[start - 1])) start--
  return words.slice(start)
}

function compareKeys(words: string[]): string[] {
  return words.map((word, index) => {
    const folded = word.replace(TYPOGRAPHIC, char => ASCII_PUNCTUATION[char]).toLowerCase()
    return index === words.length - 1 ? folded.replace(TRAILING_TERMINAL, '') : folded
  })
}

function keep(words: string[]): DiffWord[] {
  return words.map(text => ({ text, changed: false }))
}

export function diffWords(userAnswer: string, correctAnswer: string): WordDiff {
  const userWords = tokenize(userAnswer)
  const correctWords = tokenize(correctAnswer)

  if (userWords.length > MAX_WORDS || correctWords.length > MAX_WORDS) {
    return { user: keep(userWords), correct: keep(correctWords) }
  }

  const userTail = trailingPunctuation(userWords)
  const correctTail = trailingPunctuation(correctWords)
  const user = userWords.slice(0, userWords.length - userTail.length)
  const correct = correctWords.slice(0, correctWords.length - correctTail.length)

  const userKeys = compareKeys(user)
  const correctKeys = compareKeys(correct)
  const same = (i: number, j: number) => userKeys[i] === correctKeys[j]

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

  return {
    user: [...userDiff, ...keep(userTail)],
    correct: [...correctDiff, ...keep(correctTail)],
  }
}
