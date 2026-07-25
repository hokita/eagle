import { test, expect } from '@playwright/test'
import { signInAndGetSentence } from './helpers'

test('submitting the exact correct answer shows correct feedback', async ({ page }) => {
  const sentence = await signInAndGetSentence(page)

  const initialCorrectText = await page.getByText(/^Correct: \d+$/).textContent()
  const initialCorrectCount = Number(initialCorrectText?.replace('Correct: ', ''))

  await page.getByLabel('Your English translation').fill(sentence.english)
  await page.getByRole('button', { name: 'Check Translation' }).click()

  await expect(page.getByText('Correct! Well done!')).toBeVisible()
  await expect(page.getByText(`Correct: ${initialCorrectCount + 1}`)).toBeVisible()
})
