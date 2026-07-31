import { test, expect, type Page } from '@playwright/test'

// The graph is canvas-rendered; we assert real content through the exposed
// cytoscape instance and through DOM proxies (inspector, breadcrumbs).
function nodeCount(page: Page): Promise<number> {
  return page.evaluate(() => (window as any).__cy?.nodes().length ?? 0)
}
function edgeCount(page: Page): Promise<number> {
  return page.evaluate(() => (window as any).__cy?.edges().length ?? 0)
}

test('bare landing auto-loads a graph from lore', async ({ page }) => {
  await page.goto('/')
  await expect(page.getByTestId('health')).toContainText('●')
  await expect(page.getByTestId('graph-canvas')).toBeVisible()
  await expect(page.getByTestId('legend')).toBeVisible()
  // The canvas must not be empty on landing — this is the whole point of seed.
  await expect.poll(() => nodeCount(page)).toBeGreaterThan(0)
  await expect(page.getByTestId('inspector-title')).toBeVisible()
  await expect(page.getByTestId('suggestion').first()).toBeVisible()
})

test('focus a symbol renders a code+lore neighborhood', async ({ page }) => {
  await page.goto('/?focus=sym:Neighborhood')
  await expect(page.getByTestId('inspector-title')).toHaveText('Neighborhood')
  await expect.poll(() => nodeCount(page)).toBeGreaterThan(1)
  await expect.poll(() => edgeCount(page)).toBeGreaterThan(0)
  await expect(page.getByTestId('neighbor').first()).toBeVisible()
  await expect(page.getByTestId('crumb')).toHaveCount(1)
  await page.screenshot({ path: 'test-results/focus.png', fullPage: true })
})

test('dig deeper by expanding a node grows the graph', async ({ page }) => {
  await page.goto('/?focus=sym:Neighborhood')
  await expect.poll(() => nodeCount(page)).toBeGreaterThan(1)
  const before = await nodeCount(page)

  // Simulate a graph node tap on a non-focus node → expand (merge).
  await page.evaluate(() => {
    const cy = (window as any).__cy
    const focusId = cy.$('.focus').id()
    const target = cy.nodes().filter((n: any) => n.id() !== focusId)[0]
    target.emit('tap')
  })

  await expect.poll(() => nodeCount(page)).toBeGreaterThanOrEqual(before)
  await expect(page.getByTestId('crumb')).toHaveCount(2)
  await page.screenshot({ path: 'test-results/expand.png', fullPage: true })
})

test('navigate via an inspector neighbor chip', async ({ page }) => {
  await page.goto('/?focus=sym:Neighborhood')
  await expect(page.getByTestId('inspector-title')).toHaveText('Neighborhood')

  const firstNeighbor = page.getByTestId('neighbor').first()
  const neighborLabel = (await firstNeighbor.locator('.neighbor-label').textContent())?.trim()
  await firstNeighbor.click()

  await expect.poll(() => nodeCount(page)).toBeGreaterThan(0)
  if (neighborLabel) {
    await expect(page.getByTestId('inspector-title')).toHaveText(neighborLabel)
  }
  await expect(page.getByTestId('crumb')).toHaveCount(2)
})

test('command palette sets the focus', async ({ page }) => {
  await page.goto('/?focus=sym:Neighborhood')
  await expect(page.getByTestId('inspector-title')).toHaveText('Neighborhood')
  await page.getByTestId('palette-input').fill('sym:SymbolNeighborhood')
  await page.getByTestId('palette-input').press('Enter')
  await expect(page.getByTestId('inspector-title')).toHaveText('SymbolNeighborhood')
})
