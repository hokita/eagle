import { test, expect } from '@playwright/test'
import { signInAndGetSentence } from './helpers'

const QUESTION =
  'Who should take more responsibility for environmental problems: individuals, companies, or governments?'

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
  await expect(page.getByText('日本語で答えるなら、他に言いたかったことはありますか？')).toBeVisible()
  await page.getByLabel('Japanese reflection').fill('制度そのものを変える必要があると思う。')
  await page.getByRole('button', { name: 'Submit' }).click()

  // Study phase shows the stub analysis, corrections included.
  await expect(page.getByText('Systemic change is more effective than individual action.')).toBeVisible()
  await expect(page.getByText('take responsibility for').first()).toBeVisible()
  await expect(page.getByText('I think companies are responsible.')).toBeVisible()
  await page.getByRole('button', { name: 'Try the question again' }).click()

  // Retry and comparison.
  await page
    .getByLabel('Your improved answer')
    .fill('Companies should take responsibility for their impact and make systemic changes.')
  await page.getByRole('button', { name: 'Submit answer' }).click()
  await expect(page.getByText('This is a stub retry feedback for e2e tests.')).toBeVisible()
  await expect(page.getByText('I think companies.')).toBeVisible()

  // History shows the saved session.
  await page.getByRole('link', { name: 'View history' }).click()
  await expect(page.getByRole('heading', { name: 'Discussion History' })).toBeVisible()
  await expect(page.getByText(QUESTION)).toBeVisible()
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

  // The reflection is required too — it is what the analysis is built from.
  await expect(page.getByLabel('Japanese reflection')).toBeVisible()
  await expect(page.getByRole('button', { name: 'Nothing to add — skip' })).toHaveCount(0)
  await expect(page.getByRole('button', { name: 'Submit' })).toBeDisabled()
})
