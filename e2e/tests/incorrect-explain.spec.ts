import { test, expect } from '@playwright/test'
import { signInAndGetSentence } from './helpers'

test('submitting a wrong answer shows incorrect feedback, then Explain works', async ({ page }) => {
  const sentence = await signInAndGetSentence(page)

  const initialIncorrectText = await page.getByText(/^Incorrect: \d+$/).textContent()
  const initialIncorrectCount = Number(initialIncorrectText?.replace('Incorrect: ', ''))

  await page.getByLabel('Your English translation').fill('This is definitely the wrong answer')
  await page.getByRole('button', { name: 'Check Translation' }).click()

  await expect(page.getByText('Not quite right. Try again!')).toBeVisible()
  await expect(page.getByText(`Incorrect: ${initialIncorrectCount + 1}`)).toBeVisible()
  await expect(page.getByText(sentence.english)).toBeVisible()

  await page.getByRole('button', { name: 'Explain' }).click()
  await expect(page.getByText('This is a stub explanation for e2e tests.')).toBeVisible()

  // The answer stays on screen next to the explanation — no tab switching.
  await expect(page.getByText(sentence.english)).toBeVisible()
})

test('a free-text question about the explanation is answered in a thread', async ({ page }) => {
  await signInAndGetSentence(page)

  await page.getByLabel('Your English translation').fill('This is definitely the wrong answer')
  await page.getByRole('button', { name: 'Check Translation' }).click()
  await expect(page.getByText('Not quite right. Try again!')).toBeVisible()

  // Nothing to ask about until there is an explanation on screen.
  await expect(page.getByLabel('Your question about this explanation')).toBeHidden()

  await page.getByRole('button', { name: 'Explain' }).click()
  await expect(page.getByText('This is a stub explanation for e2e tests.')).toBeVisible()

  const question = page.getByLabel('Your question about this explanation')
  await question.fill('Why is my answer wrong?')
  await page.getByRole('button', { name: 'Ask' }).click()

  await expect(page.getByText('Stub answer to: Why is my answer wrong?')).toBeVisible()
  // The question stays on screen above its answer, and the box is empty for
  // the next one.
  await expect(page.getByText('Why is my answer wrong?', { exact: true })).toBeVisible()
  await expect(question).toHaveValue('')

  await question.fill('What should I have written?')
  await page.getByRole('button', { name: 'Ask' }).click()

  await expect(page.getByText('Stub answer to: What should I have written?')).toBeVisible()
  await expect(page.getByText('Stub answer to: Why is my answer wrong?')).toBeVisible()
})
