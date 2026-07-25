import { test, expect } from '@playwright/test'
import { signInAndGetSentence } from './helpers'

test('shows the login screen when signed out', async ({ page }) => {
  await page.goto('/')
  await expect(page.getByRole('button', { name: 'Sign in with Google' })).toBeVisible()
})

test('signs in and lands on the Translator with a sentence loaded', async ({ page }) => {
  const sentence = await signInAndGetSentence(page)
  await expect(page.getByText(sentence.japanese)).toBeVisible()
})

test('signs out and returns to the login screen', async ({ page }) => {
  await signInAndGetSentence(page)
  await page.locator('img.cursor-pointer').click()
  await page.getByRole('menuitem', { name: 'Sign out' }).click()
  await expect(page.getByRole('button', { name: 'Sign in with Google' })).toBeVisible()
})
