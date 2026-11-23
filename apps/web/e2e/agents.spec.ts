import { expect, test } from '@playwright/test'

test.use({ userAgent: 'Mozilla/5.0 (Linux; Android 15) AppleWebKit/537.36 Mobile Safari/537.36' })

test('administrator can view all agent downloads and manage an enrollment token from a mobile browser', async ({ page, request }) => {
  const { CRYOL_E2E_USERNAME: username, CRYOL_E2E_PASSWORD: password } = process.env
  if (!username || !password) throw new Error('Configure disposable Cryol administrator credentials.')
  expect((await request.get('/api/v1/agents')).status()).toBe(401)
  await page.goto('/console/agents')
  await page.getByLabel('Username').fill(username)
  await page.getByLabel('Password', { exact: true }).fill(password)
  await page.getByRole('button', { name: 'Sign in' }).click()
  await expect(page.getByRole('heading', { name: 'Endpoint agents' })).toBeVisible()
  const trigger = page.getByRole('button', { name: 'Agent downloads' })
  const enrollment = page.getByRole('dialog', { name: 'Agent downloads' })
  await expect(enrollment).toBeHidden()
  await trigger.click()
  await expect(enrollment).toBeVisible()
  await expect(enrollment.getByLabel('Endpoint operating system')).toHaveCount(0)
  if (process.env.CRYOL_E2E_MAC_DOWNLOAD === '1') {
    await expect(enrollment.getByRole('link', { name: 'Download macOS Apple silicon ZIP' })).toBeVisible()
    await expect(enrollment.getByRole('link', { name: 'Download Linux ARM64 ZIP' })).toBeVisible()
    await expect(enrollment.getByRole('link', { name: 'Download Linux x64 ZIP' })).toBeVisible()
    const download = page.waitForEvent('download')
    await enrollment.getByRole('link', { name: 'Download macOS Intel ZIP' }).click()
    expect((await download).suggestedFilename()).toBe('cryol-agent-macos-intel.zip')
  }
  if (process.env.CRYOL_E2E_INSTALLER === '1') {
    const download = page.waitForEvent('download')
    await enrollment.getByRole('link', { name: 'Download Windows x64 installer' }).click()
    expect((await download).suggestedFilename()).toBe('Cryol-Agent-Setup-x64.exe')
    await expect(enrollment.getByRole('link', { name: 'Download Windows x64 ZIP' })).toHaveCount(0)
    await expect(enrollment.getByText('Unsigned test build.')).toBeVisible()
  } else if (process.env.CRYOL_E2E_SETUP === '1') {
    const download = page.waitForEvent('download')
    await enrollment.getByRole('link', { name: 'Download Windows x64 ZIP' }).click()
    expect((await download).suggestedFilename()).toBe('cryol-agent-windows-service-x64.zip')
    await expect(enrollment.getByText('This download is for a test agent on this computer.')).toBeVisible()
  }
  await enrollment.getByRole('button', { name: 'Generate token' }).click()
  await expect(enrollment.getByLabel('Enrollment token')).toBeVisible()
  await page.keyboard.press('Escape')
  await expect(trigger).toBeFocused()
  await trigger.click()
  await expect(enrollment.getByLabel('Enrollment token')).toBeVisible()
  await enrollment.getByRole('button', { name: 'Revoke unused token' }).click()
  await expect(enrollment.getByLabel('Enrollment token')).toBeHidden()
})
