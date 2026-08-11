import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { speakJapanese } from './speech'

class FakeUtterance {
  lang = ''
  rate = 0
  pitch = 0
  volume = 0
  voice: unknown = null
  onstart: (() => void) | null = null
  onend: (() => void) | null = null
  onerror: (() => void) | null = null
  constructor(public text: string) {}
}

let spoken: FakeUtterance[]
let synth: {
  speak: ReturnType<typeof vi.fn>
  cancel: ReturnType<typeof vi.fn>
  getVoices: ReturnType<typeof vi.fn>
  speaking: boolean
  pending: boolean
}

beforeEach(() => {
  vi.useFakeTimers()
  spoken = []
  synth = {
    speak: vi.fn((u: FakeUtterance) => spoken.push(u)),
    cancel: vi.fn(),
    getVoices: vi.fn(() => [{ lang: 'en-US' }, { lang: 'ja-JP' }]),
    speaking: false,
    pending: false,
  }
  vi.stubGlobal('speechSynthesis', synth)
  vi.stubGlobal('SpeechSynthesisUtterance', FakeUtterance)
})

afterEach(() => {
  vi.useRealTimers()
  vi.unstubAllGlobals()
})

describe('speakJapanese', () => {
  it('cancels any in-flight speech and reports speaking immediately', () => {
    const onSpeakingChange = vi.fn()
    speakJapanese('時間がありません。', onSpeakingChange)

    expect(synth.cancel).toHaveBeenCalled()
    expect(onSpeakingChange).toHaveBeenCalledWith(true)
  })

  it('speaks the text with a Japanese voice and ja-JP locale', () => {
    speakJapanese('時間がありません。', vi.fn())
    vi.advanceTimersByTime(100)

    expect(spoken).toHaveLength(1)
    expect(spoken[0].text).toBe('時間がありません。')
    expect(spoken[0].lang).toBe('ja-JP')
    expect(spoken[0].voice).toEqual({ lang: 'ja-JP' })
  })

  it('reports not-speaking when the utterance ends', () => {
    const onSpeakingChange = vi.fn()
    speakJapanese('テスト', onSpeakingChange)
    vi.advanceTimersByTime(100)

    spoken[0].onend?.()

    expect(onSpeakingChange).toHaveBeenLastCalledWith(false)
  })

  it('reports not-speaking when the utterance errors', () => {
    const onSpeakingChange = vi.fn()
    speakJapanese('テスト', onSpeakingChange)
    vi.advanceTimersByTime(100)

    spoken[0].onerror?.()

    expect(onSpeakingChange).toHaveBeenLastCalledWith(false)
  })

  it('force-resets after 500ms when Safari silently drops the utterance', () => {
    const onSpeakingChange = vi.fn()
    speakJapanese('テスト', onSpeakingChange)
    vi.advanceTimersByTime(100)

    synth.speaking = false
    synth.pending = false
    vi.advanceTimersByTime(500)

    expect(onSpeakingChange).toHaveBeenLastCalledWith(false)
  })

  it('does not force-reset while speech is genuinely in flight', () => {
    const onSpeakingChange = vi.fn()
    speakJapanese('テスト', onSpeakingChange)
    vi.advanceTimersByTime(100)
    onSpeakingChange.mockClear()

    synth.speaking = true
    vi.advanceTimersByTime(500)

    expect(onSpeakingChange).not.toHaveBeenCalledWith(false)
  })

  it('does nothing when the browser has no speech synthesis', () => {
    vi.unstubAllGlobals()
    const onSpeakingChange = vi.fn()

    expect(() => speakJapanese('テスト', onSpeakingChange)).not.toThrow()
    expect(onSpeakingChange).not.toHaveBeenCalled()
  })
})
