import { test, expect } from '@playwright/test'
import { signInAndGetSentence } from './helpers'

const QUESTION =
  'Who should take more responsibility for environmental problems: individuals, companies, or governments?'

const STUB_SUMMARY =
  'I think companies are responsible, and in the future they should make systemic changes.'

test('completes a discussion session end to end', async ({ page }) => {
  await signInAndGetSentence(page)

  await page.getByRole('link', { name: 'Discussion' }).click()
  await expect(page.getByText(QUESTION)).toBeVisible()

  // Initial answer + two stub follow-ups, then the stub closes the conversation.
  await page.getByLabel('Your answer').fill('I think companies.')
  await page.getByRole('button', { name: 'Send' }).click()
  await expect(page.getByText('Stub follow-up 1: can you tell me more?')).toBeVisible()

  await page.getByLabel('Your answer').fill('Because companies affect environment more.')
  await page.getByRole('button', { name: 'Send' }).click()
  await expect(page.getByText('Stub follow-up 2: can you tell me more?')).toBeVisible()

  await page.getByLabel('Your answer').fill('They can change their products.')
  await page.getByRole('button', { name: 'Send' }).click()

  // Reflection phase: the server ends the conversation after the second
  // follow-up is answered, with no closing line of its own.
  await expect(page.getByText('What else did you want to say? (in Japanese)')).toBeVisible()
  await page.getByLabel('Japanese reflection').fill('制度そのものを変える必要があると思う。')
  await page.getByRole('button', { name: 'Finish' }).click()

  // The summary is the last screen: the natural rewrite plus the phrases.
  await expect(page.getByText('Natural English')).toBeVisible()
  await expect(page.getByText(STUB_SUMMARY)).toBeVisible()
  await expect(page.getByText('Useful phrases')).toBeVisible()
  await expect(page.getByText('take responsibility for').first()).toBeVisible()
  await expect(page.getByText('in the future').first()).toBeVisible()

  // History shows the saved session, summary included.
  await page.getByRole('link', { name: 'View history' }).click()
  await expect(page.getByRole('heading', { name: 'Discussion History' })).toBeVisible()
  await page.getByRole('button', { name: QUESTION }).click()
  await expect(page.getByText(STUB_SUMMARY)).toBeVisible()
})

test('starting a new question resets the session', async ({ page }) => {
  await signInAndGetSentence(page)

  await page.getByRole('link', { name: 'Discussion' }).click()
  await expect(page.getByText(QUESTION)).toBeVisible()

  for (const answer of ['I think companies.', 'Because they pollute more.', 'They can change.']) {
    await page.getByLabel('Your answer').fill(answer)
    await page.getByRole('button', { name: 'Send' }).click()
  }

  await page.getByLabel('Japanese reflection').fill('制度そのものを変える必要があると思う。')
  await page.getByRole('button', { name: 'Finish' }).click()
  await expect(page.getByText(STUB_SUMMARY)).toBeVisible()

  await page.getByRole('button', { name: 'Next question' }).click()
  await expect(page.getByLabel('Your answer')).toBeVisible()
  await expect(page.getByText(STUB_SUMMARY)).toHaveCount(0)
})

test('no step of the session can be skipped', async ({ page }) => {
  await signInAndGetSentence(page)

  await page.getByRole('link', { name: 'Discussion' }).click()
  await expect(page.getByText(QUESTION)).toBeVisible()

  // The conversation runs a fixed two follow-ups with no way out of it.
  await expect(page.getByRole('button', { name: 'Finish conversation' })).toHaveCount(0)

  await page.getByLabel('Your answer').fill('I think governments.')
  await page.getByRole('button', { name: 'Send' }).click()
  await expect(page.getByText('Stub follow-up 1: can you tell me more?')).toBeVisible()
  await expect(page.getByRole('button', { name: 'Finish conversation' })).toHaveCount(0)

  await page.getByLabel('Your answer').fill('Because they make the rules.')
  await page.getByRole('button', { name: 'Send' }).click()
  await expect(page.getByText('Stub follow-up 2: can you tell me more?')).toBeVisible()

  await page.getByLabel('Your answer').fill('They can pass stricter laws.')
  await page.getByRole('button', { name: 'Send' }).click()

  // The reflection is required too — it is what the summary is built from.
  await expect(page.getByLabel('Japanese reflection')).toBeVisible()
  await expect(page.getByRole('button', { name: 'Nothing to add — skip' })).toHaveCount(0)
  await expect(page.getByRole('button', { name: 'Finish' })).toBeDisabled()
})
