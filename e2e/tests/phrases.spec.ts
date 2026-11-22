import { test, expect, type Page } from '@playwright/test'
import { signInAndGetSentence } from './helpers'

test('a phrase saved after checking an answer shows on the Phrases page with its meaning and examples', async ({
  page,
}) => {
  const sentence = await signInAndGetSentence(page)
  const phrase = sentence.english.replace(/\.$/, '').split(' ').slice(0, 2).join(' ')

  await page.getByLabel('Your English translation').fill(sentence.english)
  await page.getByRole('button', { name: 'Check Translation' }).click()
  await expect(page.getByText('Correct! Well done!')).toBeVisible()

  await page.getByLabel('Phrase', { exact: true }).fill(phrase)
  await page.getByRole('button', { name: 'Save Phrase' }).click()
  const saved = page.getByRole('list', { name: 'Saved phrases' })
  await expect(saved).toContainText(phrase)
  await expect(saved).toContainText(`Stub meaning of ${phrase}`)

  await page.getByRole('link', { name: 'Phrases' }).click()
  await expect(page).toHaveURL(/\/phrases/)
  await expect(page.getByRole('heading', { name: phrase })).toBeVisible()
  await expect(page.getByText(`Stub meaning of ${phrase}`)).toBeVisible()
  const examples = page.getByRole('list', { name: `Examples for ${phrase}` })
  await expect(examples.getByRole('listitem')).toHaveCount(3)
  await expect(examples).toContainText(`Stub example 1 with ${phrase}.`)
  await expect(page.getByText(sentence.japanese)).toHaveCount(0)

  await page.getByRole('button', { name: 'New examples' }).click()
  await expect(examples.getByRole('listitem')).toHaveCount(3)

  await reopenPhrases(page)
  await expect(page.getByRole('heading', { name: phrase })).toBeVisible()

  await page.getByRole('button', { name: `Delete ${phrase}` }).click()
  await expect(page.getByText('Delete this phrase?')).toBeVisible()
  await page.getByRole('button', { name: 'Delete', exact: true }).click()
  await expect(page.getByText('No phrases yet')).toBeVisible()

  await reopenPhrases(page)
  await expect(page.getByText('No phrases yet')).toBeVisible()
})

async function reopenPhrases(page: Page) {
  await page.getByRole('link', { name: 'Eagle' }).click()
  await expect(page.getByLabel('Your English translation')).toBeVisible()
  await page.getByRole('link', { name: 'Phrases' }).click()
  await expect(page).toHaveURL(/\/phrases/)
}
