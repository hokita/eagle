export function speakJapanese(
  text: string,
  onSpeakingChange: (speaking: boolean) => void
): void {
  if (!('speechSynthesis' in window)) return

  speechSynthesis.cancel()
  onSpeakingChange(true)

  setTimeout(() => {
    const utterance = new SpeechSynthesisUtterance(text)

    const japaneseVoice = speechSynthesis
      .getVoices()
      .find(voice => voice.lang.startsWith('ja') || voice.lang.includes('JP'))
    if (japaneseVoice) {
      utterance.voice = japaneseVoice
    }

    utterance.lang = 'ja-JP'
    utterance.rate = 0.8
    utterance.pitch = 1
    utterance.volume = 1

    utterance.onstart = () => onSpeakingChange(true)
    utterance.onend = () => onSpeakingChange(false)
    utterance.onerror = () => onSpeakingChange(false)

    speechSynthesis.speak(utterance)

    setTimeout(() => {
      if (!speechSynthesis.speaking && !speechSynthesis.pending) {
        onSpeakingChange(false)
      }
    }, 500)
  }, 100)
}
