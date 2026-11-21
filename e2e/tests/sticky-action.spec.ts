import { test, expect } from '@playwright/test'
import { signInAndGetSentence } from './helpers'

test.use({ viewport: { width: 390, height: 844 } })

test('the action row sits right below the content when it fits the viewport', async ({ page }) => {
  await signInAndGetSentence(page)

  const checkButton = page.getByRole('button', { name: 'Check Translation' })
  await checkButton.waitFor()

  const gap = await checkButton.evaluate(button => {
    const actionRow = button.parentElement as HTMLElement
    const content = actionRow.previousElementSibling as HTMLElement
    return actionRow.getBoundingClientRect().top - content.getBoundingClientRect().bottom
  })
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
