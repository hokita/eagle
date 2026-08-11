import { test, expect } from '@playwright/test'
import { signInAndGetSentence } from './helpers'

// Phone-sized viewport: this is the case that broke. A 2546-character AI
// explanation once pushed "Next Sentence" 1138px below the fold.
test.use({ viewport: { width: 390, height: 844 } })

test('the action row sits right below the content when it fits the viewport', async ({ page }) => {
  await signInAndGetSentence(page)

  const checkButton = page.getByRole('button', { name: 'Check Translation' })
  await checkButton.waitFor()

  // The action row must follow the card holding the input form, not float at
  // the bottom of the window with a gap in between.
  const gap = await checkButton.evaluate(button => {
    const actionRow = button.parentElement as HTMLElement
    const content = actionRow.previousElementSibling as HTMLElement
    return actionRow.getBoundingClientRect().top - content.getBoundingClientRect().bottom
  })
  // Bounded on both sides: a negative gap would mean the row overlaps the card.
  expect(gap).toBeGreaterThanOrEqual(0)
  expect(gap).toBeLessThanOrEqual(16)
})

test('the action row stays reachable when review content overflows the viewport', async ({ page }) => {
  await signInAndGetSentence(page)

  await page.getByLabel('Your English translation').fill('This is definitely the wrong answer')
  await page.getByRole('button', { name: 'Check Translation' }).click()
  await expect(page.getByText('Not quite right. Try again!')).toBeVisible()

  const nextButton = page.getByRole('button', { name: 'Next Sentence' })
  await nextButton.waitFor()

  // Simulate a very long AI explanation by growing the content above the
  // action row well past the viewport height, mirroring the real defect.
  await nextButton.evaluate(button => {
    const actionRow = button.parentElement?.parentElement as HTMLElement | null
    const content = actionRow?.previousElementSibling as HTMLElement | null
    if (!content) throw new Error('could not locate the content container to overflow')
    const spacer = document.createElement('div')
    spacer.style.height = '2600px'
    spacer.setAttribute('data-testid', 'e2e-overflow-spacer')
    content.appendChild(spacer)
  })

  const actionRowPosition = await nextButton.evaluate(button => {
    const actionRow = button.parentElement?.parentElement as HTMLElement
    return getComputedStyle(actionRow).position
  })
  expect(actionRowPosition).toBe('sticky')

  const { bottom } = await nextButton.evaluate(button => button.getBoundingClientRect())
  const viewportHeight = await page.evaluate(() => window.innerHeight)
  expect(bottom).toBeLessThanOrEqual(viewportHeight)
})
