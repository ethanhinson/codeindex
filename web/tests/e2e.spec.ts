import { test, expect, type Page } from '@playwright/test'

function nodeCount(page: Page): Promise<number> {
  return page.evaluate(() => (window as any).__cy?.nodes().length ?? 0)
}
function selectedCount(page: Page): Promise<number> {
  return page.evaluate(() => (window as any).__cy?.$('.sel').length ?? 0)
}

test('landing loads the whole project graph', async ({ page }) => {
  await page.goto('/')
  await expect(page.getByTestId('graph-canvas')).toBeVisible()
  await expect(page.getByTestId('legend')).toBeVisible()
  await expect(page.getByTestId('health')).toContainText('symbols')
  // The whole graph is thousands of nodes, not a handful.
  await expect.poll(() => nodeCount(page), { timeout: 15000 }).toBeGreaterThan(500)
})

test('search selects a symbol and inspects it', async ({ page }) => {
  await page.goto('/')
  await expect.poll(() => nodeCount(page), { timeout: 15000 }).toBeGreaterThan(500)
  await page.getByTestId('palette-input').fill('Neighborhood')
  await page.getByTestId('palette-input').press('Enter')

  await expect(page.getByTestId('inspector-title')).toHaveText('Neighborhood')
  await expect.poll(() => selectedCount(page)).toBe(1)
  await expect(page.getByTestId('neighbor').first()).toBeVisible()
  await page.screenshot({ path: 'test-results/selected.png', fullPage: true })
})

test('deep link ?focus= selects on load', async ({ page }) => {
  await page.goto('/?focus=SymbolNeighborhood')
  await expect.poll(() => nodeCount(page), { timeout: 15000 }).toBeGreaterThan(500)
  await expect(page.getByTestId('inspector-title')).toHaveText('SymbolNeighborhood', { timeout: 15000 })
  await expect.poll(() => selectedCount(page)).toBe(1)
})

test('clicking an inspector neighbor changes selection', async ({ page }) => {
  await page.goto('/?focus=Neighborhood')
  await expect.poll(() => nodeCount(page), { timeout: 15000 }).toBeGreaterThan(500)
  await expect(page.getByTestId('inspector-title')).toHaveText('Neighborhood', { timeout: 15000 })
  const neighbor = page.getByTestId('neighbor').first()
  const label = (await neighbor.locator('.neighbor-label').textContent())?.trim()
  await neighbor.click()
  if (label) {
    await expect(page.getByTestId('inspector-title')).toHaveText(label)
  }
  await expect.poll(() => selectedCount(page)).toBe(1)
})
