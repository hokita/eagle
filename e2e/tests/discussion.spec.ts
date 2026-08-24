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

  // Reflection phase (stub closing line is shown above the prompt).
  await expect(page.getByText('日本語で答えるなら、他に言いたかったことはありますか？')).toBeVisible()
  await expect(page.getByText('Great, thanks for sharing your thoughts!')).toBeVisible()
  await page.getByLabel('Japanese reflection').fill('制度そのものを変える必要があると思う。')
  await page.getByRole('button', { name: 'Submit' }).click()

  // Study phase shows the stub analysis.
  await expect(page.getByText('Systemic change is more effective than individual action.')).toBeVisible()
  await expect(page.getByText('take responsibility for').first()).toBeVisible()
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

test('reflection can be skipped', async ({ page }) => {
  await signInAndGetSentence(page)

  await page.getByRole('link', { name: 'Discussion' }).click()
  await expect(page.getByText(QUESTION)).toBeVisible()

  await page.getByLabel('Your answer').fill('I think governments.')
  await page.getByRole('button', { name: 'Send' }).click()
  await expect(page.getByText('Stub follow-up 1: can you tell me more?')).toBeVisible()

  await page.getByLabel('Your answer').fill('Because they make the rules.')
  await page.getByRole('button', { name: 'Send' }).click()
  await expect(page.getByText('Stub follow-up 2: can you tell me more?')).toBeVisible()

  // Finish early instead of answering the second follow-up.
  await page.getByRole('button', { name: 'Finish conversation' }).click()
  await expect(page.getByText('日本語で答えるなら、他に言いたかったことはありますか？')).toBeVisible()
  await page.getByRole('button', { name: 'Nothing to add — skip' }).click()

  await page.getByLabel('Your improved answer').fill('I still think governments, because they set the rules.')
  await page.getByRole('button', { name: 'Submit answer' }).click()
  await expect(page.getByText('This is a stub retry feedback for e2e tests.')).toBeVisible()
})
