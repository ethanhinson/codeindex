import { test, expect, type Page } from '@playwright/test'

declare global {
  interface Window {
    __cy?: any
    __layoutDone?: boolean
  }
}

const ready = (page: Page) =>
  expect.poll(() => page.evaluate(() => window.__layoutDone === true), { timeout: 20000 }).toBe(true)

const count = (page: Page, selector: string) =>
  page.evaluate((sel) => window.__cy?.$(sel).length ?? 0, selector)

// Largest package by symCount — stable target for expand/chip tests.
const biggestPackage = (page: Page) =>
  page.evaluate(() => {
    const pkgs = window.__cy.$('node[kind = "package"]')
    let best: any = null
    pkgs.forEach((n: any) => {
      if (!best || n.data('symCount') > best.data('symCount')) best = n
    })
    return { label: best.data('label') as string, symCount: best.data('symCount') as number }
  })

const tapNode = (page: Page, id: string) =>
  page.evaluate((nid) => window.__cy.$id(nid).emit('tap'), id)

test('landing is an overview: packages + lore, zero symbols', async ({ page }) => {
  await page.goto('/')
  await expect(page.getByTestId('graph-canvas')).toBeVisible()
  await ready(page)
  await expect(page.getByTestId('health')).toContainText('packages')
  expect(await count(page, 'node[kind = "package"]')).toBeGreaterThan(10)
  expect(await count(page, 'node[kind = "symbol"]')).toBe(0)
  // Bundled edges connect the packages.
  expect(await count(page, 'edge[?bundled]')).toBeGreaterThan(5)
})

test('expand shows top-12 + chip; chip reveals the tail; collapse re-bundles', async ({ page }) => {
  await page.goto('/')
  await ready(page)
  const pkg = await biggestPackage(page)
  expect(pkg.symCount).toBeGreaterThan(12)

  await tapNode(page, `pkg:${pkg.label}`)
  await expect.poll(() => count(page, 'node[kind = "symbol"]')).toBe(12)
  expect(await count(page, 'node[kind = "chip"]')).toBe(1)
  const chipLabel = await page.evaluate(() => window.__cy.$('node[kind = "chip"]').data('label'))
  expect(chipLabel).toBe(`+${pkg.symCount - 12} more`)

  await tapNode(page, `chip:${pkg.label}`)
  await expect.poll(() => count(page, 'node[kind = "symbol"]')).toBe(pkg.symCount)
  expect(await count(page, 'node[kind = "chip"]')).toBe(0)

  await tapNode(page, `pkg:${pkg.label}`)
  await expect.poll(() => count(page, 'node[kind = "symbol"]')).toBe(0)
})

test('expansion never moves existing nodes', async ({ page }) => {
  await page.goto('/')
  await ready(page)
  const before = await page.evaluate(() => {
    const o: Record<string, { x: number; y: number }> = {}
    window.__cy.$('node[kind = "package"]').forEach((n: any) => {
      o[n.id()] = { x: Math.round(n.position('x')), y: Math.round(n.position('y')) }
    })
    return o
  })
  const pkg = await biggestPackage(page)
  await tapNode(page, `pkg:${pkg.label}`)
  await expect.poll(() => count(page, 'node[kind = "symbol"]')).toBeGreaterThan(0)
  const after = await page.evaluate(() => {
    const o: Record<string, { x: number; y: number }> = {}
    window.__cy.$('node[kind = "package"]').forEach((n: any) => {
      o[n.id()] = { x: Math.round(n.position('x')), y: Math.round(n.position('y')) }
    })
    return o
  })
  // Every collapsed package sits exactly where it was (the expanded one may
  // grow as a compound, so skip it).
  for (const id of Object.keys(before)) {
    if (id === `pkg:${pkg.label}`) continue
    expect(after[id], id).toEqual(before[id])
  }
})

test('layout is deterministic across reloads', async ({ page }) => {
  const positions = async () => {
    await page.goto('/')
    await ready(page)
    return page.evaluate(() => {
      const o: Record<string, { x: number; y: number }> = {}
      window.__cy.$('node[kind = "package"]').forEach((n: any) => {
        o[n.id()] = { x: Math.round(n.position('x')), y: Math.round(n.position('y')) }
      })
      return o
    })
  }
  const first = await positions()
  const second = await positions()
  expect(second).toEqual(first)
})

test('search auto-expands the package and selects the symbol', async ({ page }) => {
  await page.goto('/')
  await ready(page)
  await page.getByTestId('palette-input').fill('Neighborhood')
  await page.getByTestId('palette-input').press('Enter')

  await expect(page.getByTestId('inspector-title')).toHaveText('Neighborhood')
  await expect.poll(() => count(page, 'node.sel')).toBe(1)
  // Its package is now an expanded compound holding the symbol.
  const inPkg = await page.evaluate(() => {
    const n = window.__cy.$('node.sel')
    return n.parent().data('kind') === 'package'
  })
  expect(inPkg).toBe(true)
  await expect(page.getByTestId('neighbor').first()).toBeVisible()
})

test('deep link ?focus= reveals and selects on load', async ({ page }) => {
  await page.goto('/?focus=SymbolNeighborhood')
  await ready(page)
  await expect(page.getByTestId('inspector-title')).toHaveText('SymbolNeighborhood', { timeout: 15000 })
  await expect.poll(() => count(page, 'node.sel')).toBe(1)
})

test('clicking an inspector neighbor changes selection', async ({ page }) => {
  await page.goto('/?focus=Neighborhood')
  await ready(page)
  await expect(page.getByTestId('inspector-title')).toHaveText('Neighborhood', { timeout: 15000 })
  const neighbor = page.getByTestId('neighbor').first()
  const label = (await neighbor.locator('.neighbor-label').textContent())?.trim()
  await neighbor.click()
  if (label) {
    await expect(page.getByTestId('inspector-title')).toHaveText(label)
  }
  await expect.poll(() => count(page, 'node.sel')).toBe(1)
})
