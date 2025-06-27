"use client"

import { useState, useEffect } from "react"
import { Button } from "@/components/ui/button"
import { Card, CardContent, CardDescription, CardFooter, CardHeader, CardTitle } from "@/components/ui/card"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { CheckCircle, XCircle, BookOpen } from "lucide-react"
import { Alert, AlertDescription } from "@/components/ui/alert"

interface Sentence {
  japanese: string
  english: string
  romaji: string
}

const sentences: Sentence[] = [
  {
    japanese: "今日は天気がいいですね。",
    english: "The weather is nice today.",
    romaji: "Kyou wa tenki ga ii desu ne.",
  },
  {
    japanese: "私は学生です。",
    english: "I am a student.",
    romaji: "Watashi wa gakusei desu.",
  },
  {
    japanese: "この本はとても面白いです。",
    english: "This book is very interesting.",
    romaji: "Kono hon wa totemo omoshiroi desu.",
  },
  {
    japanese: "駅はどこですか？",
    english: "Where is the station?",
    romaji: "Eki wa doko desu ka?",
  },
  {
    japanese: "日本語を勉強しています。",
    english: "I am studying Japanese.",
    romaji: "Nihongo wo benkyou shite imasu.",
  },
  {
    japanese: "お疲れ様でした。",
    english: "Thank you for your hard work.",
    romaji: "Otsukaresama deshita.",
  },
  {
    japanese: "明日は雨が降るでしょう。",
    english: "It will probably rain tomorrow.",
    romaji: "Ashita wa ame ga furu deshou.",
  },
  {
    japanese: "コーヒーを飲みませんか？",
    english: "Would you like to drink coffee?",
    romaji: "Koohii wo nomimasen ka?",
  },
  {
    japanese: "時間がありません。",
    english: "I don't have time.",
    romaji: "Jikan ga arimasen.",
  },
  {
    japanese: "家族と一緒に住んでいます。",
    english: "I live with my family.",
    romaji: "Kazoku to issho ni sunde imasu.",
  },
]

export default function JapaneseTranslator() {
  const [currentSentence, setCurrentSentence] = useState<Sentence>(sentences[0])
  const [userTranslation, setUserTranslation] = useState("")
  const [feedback, setFeedback] = useState<"correct" | "incorrect" | null>(null)
  const [showAnswer, setShowAnswer] = useState(false)
  const [usedSentences, setUsedSentences] = useState<number[]>([])

  const getRandomSentence = () => {
    let availableSentences = sentences.filter((_, index) => !usedSentences.includes(index))

    if (availableSentences.length === 0) {
      setUsedSentences([])
      availableSentences = sentences
    }

    const randomIndex = Math.floor(Math.random() * availableSentences.length)
    const selectedSentence = availableSentences[randomIndex]
    const originalIndex = sentences.indexOf(selectedSentence)

    setUsedSentences((prev) => [...prev, originalIndex])
    setCurrentSentence(selectedSentence)
  }

  const checkTranslation = () => {
    const userAnswer = userTranslation.toLowerCase().trim()
    const correctAnswer = currentSentence.english.toLowerCase().trim()

    // Simple similarity check - in a real app, you'd use more sophisticated NLP
    const similarity = calculateSimilarity(userAnswer, correctAnswer)
    const isCorrect = similarity > 0.7 // 70% similarity threshold

    setFeedback(isCorrect ? "correct" : "incorrect")

    setShowAnswer(true)
  }

  const calculateSimilarity = (str1: string, str2: string): number => {
    const words1 = str1.split(" ")
    const words2 = str2.split(" ")
    const commonWords = words1.filter((word) => words2.includes(word))
    return commonWords.length / Math.max(words1.length, words2.length)
  }

  const nextSentence = () => {
    setUserTranslation("")
    setFeedback(null)
    setShowAnswer(false)
    getRandomSentence()
  }

  useEffect(() => {
    getRandomSentence()
  }, [])

  return (
    <div className="min-h-screen bg-gradient-to-br from-blue-50 to-indigo-100 p-4">
      <div className="max-w-2xl mx-auto">
        <div className="text-center mb-8">
          <div className="flex items-center justify-center gap-2 mb-4">
            <BookOpen className="h-8 w-8 text-indigo-600" />
            <h1 className="text-3xl font-bold text-gray-900">Japanese to English Translator</h1>
          </div>
          <p className="text-gray-600">Practice your English skills by translating Japanese sentences</p>
        </div>

        <div className="grid gap-6 mb-6">
          <Card>
            <CardHeader>
              <CardTitle>Translate this sentence</CardTitle>
              <CardDescription>Translate the Japanese sentence below into English</CardDescription>
            </CardHeader>
            <CardContent className="space-y-4">
              <div className="text-center">
                <div className="text-3xl font-bold text-gray-900 mb-2">{currentSentence.japanese}</div>
                <div className="text-sm">{currentSentence.romaji}</div>
              </div>

              <div className="space-y-2">
                <Label htmlFor="translation">Your English translation:</Label>
                <Input
                  id="translation"
                  value={userTranslation}
                  onChange={(e) => setUserTranslation(e.target.value)}
                  placeholder="Enter your translation here..."
                  disabled={showAnswer}
                  onKeyPress={(e) => {
                    if (e.key === "Enter" && userTranslation.trim() && !showAnswer) {
                      checkTranslation()
                    }
                  }}
                />
              </div>

              {feedback && (
                <Alert className={feedback === "correct" ? "border-green-500 bg-green-50" : "border-red-500 bg-red-50"}>
                  <div className="flex items-center gap-2">
                    {feedback === "correct" ? (
                      <CheckCircle className="h-4 w-4 text-green-600" />
                    ) : (
                      <XCircle className="h-4 w-4 text-red-600" />
                    )}
                    <AlertDescription className={feedback === "correct" ? "text-green-800" : "text-red-800"}>
                      {feedback === "correct" ? "Correct! Well done!" : "Not quite right. Try again!"}
                    </AlertDescription>
                  </div>
                </Alert>
              )}

              {showAnswer && (
                <div className="p-4 bg-blue-50 rounded-lg border border-blue-200">
                  <div className="font-semibold text-blue-900 mb-1">Correct Answer:</div>
                  <div className="text-blue-800">{currentSentence.english}</div>
                </div>
              )}
            </CardContent>
            <CardFooter className="flex gap-2">
              {!showAnswer ? (
                <Button onClick={checkTranslation} disabled={!userTranslation.trim()} className="flex-1">
                  Check Translation
                </Button>
              ) : (
                <Button onClick={nextSentence} className="flex-1">
                  Next Sentence
                </Button>
              )}
            </CardFooter>
          </Card>
        </div>
      </div>
    </div>
  )
}
