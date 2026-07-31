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

const anchors = (page: Page, selector: string) =>
  page.evaluate((sel) => {
    const o: Record<string, { x: number; y: number }> = {}
    window.__cy.$(sel).forEach((n: any) => {
      o[n.id()] = { x: Math.round(n.data('ax')), y: Math.round(n.data('ay')) }
    })
    return o
  }, selector)

const biggestPackage = (page: Page) =>
  page.evaluate(() => {
    const pkgs = window.__cy.$('node[kind = "package"]')
    let best: any = null
    pkgs.forEach((n: any) => {
      if (!best || n.data('symCount') > best.data('symCount')) best = n
    })
    return { label: best.data('label') as string, symCount: best.data('symCount') as number }
  })

// Reset the ready flag BEFORE triggering an in-page view change: the flag is
// only cleared inside a React effect, so polling immediately after a tap could
// otherwise observe the previous view's stale `true`.
const tapNode = (page: Page, id: string) =>
  page.evaluate((nid) => {
    window.__layoutDone = false
    window.__cy.$id(nid).emit('tap')
  }, id)

const resetReady = (page: Page) => page.evaluate(() => void (window.__layoutDone = false))

test('overview: packages only, bundled widths render, no symbols or lore on canvas', async ({ page }) => {
  await page.goto('/?motion=0')
  await expect(page.getByTestId('graph-canvas')).toBeVisible()
  await ready(page)
  await expect(page.getByTestId('health')).toContainText('packages')
  expect(await count(page, 'node[kind = "package"]')).toBeGreaterThan(10)
  expect(await count(page, 'node[kind = "symbol"]')).toBe(0)
  expect(await count(page, 'node[kind = "decision"], node[kind = "item"], node[kind = "note"]')).toBe(0)
  expect(await count(page, 'edge[?bundled]')).toBeGreaterThan(5)
  const maxBundledWidth = await page.evaluate(() =>
    Math.max(...window.__cy.$('edge[?bundled]').map((e: any) => e.numericStyle('width'))),
  )
  expect(maxBundledWidth).toBeGreaterThan(1)
})

test('focus: all symbols, satellites, ≤8 earned labels, breadcrumb + URL', async ({ page }) => {
  await page.goto('/?motion=0')
  await ready(page)
  const pkg = await biggestPackage(page)
  expect(pkg.symCount).toBeGreaterThan(8)
  await tapNode(page, `pkg:${pkg.label}`)
  await ready(page)
  await expect.poll(() => count(page, 'node[kind = "symbol"]')).toBe(pkg.symCount)
  expect(await count(page, 'node[role = "satellite"]')).toBeGreaterThan(0)
  expect(await count(page, 'node[kind = "symbol"].labeled')).toBe(8)
  await expect(page.getByTestId('crumb-pkg')).toHaveText(pkg.label)
  expect(page.url()).toContain(`pkg=${encodeURIComponent(pkg.label)}`)
})

test('satellite tap refocuses; Esc returns to the same overview map', async ({ page }) => {
  await page.goto('/?motion=0')
  await ready(page)
  const before = await anchors(page, 'node[kind = "package"]')
  const pkg = await biggestPackage(page)
  await tapNode(page, `pkg:${pkg.label}`)
  await ready(page)
  const satLabel = await page.evaluate(
    () => window.__cy.$('node[role = "satellite"]').first().data('label') as string,
  )
  await tapNode(page, `pkg:${satLabel}`)
  await ready(page)
  await expect(page.getByTestId('crumb-pkg')).toHaveText(satLabel)
  await resetReady(page)
  await page.keyboard.press('Escape')
  await ready(page)
  await expect(page.getByTestId('focus-crumb')).toHaveCount(0)
  const after = await anchors(page, 'node[kind = "package"]')
  expect(after).toEqual(before)
})

test('browser back leaves focus mode', async ({ page }) => {
  await page.goto('/?motion=0')
  await ready(page)
  const pkg = await biggestPackage(page)
  await tapNode(page, `pkg:${pkg.label}`)
  await ready(page)
  await resetReady(page)
  await page.goBack()
  await ready(page)
  await expect(page.getByTestId('focus-crumb')).toHaveCount(0)
  expect(await count(page, 'node[kind = "symbol"]')).toBe(0)
})

test('search enters focus and selects the symbol', async ({ page }) => {
  await page.goto('/?motion=0')
  await ready(page)
  await page.getByTestId('palette-input').fill('Neighborhood')
  await resetReady(page)
  await page.getByTestId('palette-input').press('Enter')
  await ready(page)
  await expect(page.getByTestId('inspector-title')).toHaveText('Neighborhood')
  await expect(page.getByTestId('focus-crumb')).toBeVisible()
  await expect.poll(() => count(page, 'node.sel')).toBe(1)
  await expect(page.getByTestId('neighbor').first()).toBeVisible()
})

test('deep link ?pkg=&focus= restores focus + selection', async ({ page }) => {
  await page.goto('/?motion=0')
  await ready(page)
  const pkg = await biggestPackage(page)
  await page.goto(`/?motion=0&pkg=${encodeURIComponent(pkg.label)}`)
  await ready(page)
  await expect(page.getByTestId('crumb-pkg')).toHaveText(pkg.label)
  expect(await count(page, 'node[kind = "symbol"]')).toBe(pkg.symCount)
})

test('lore rail: sessions hidden by default, chip reveals, hover lights the canvas', async ({ page }) => {
  await page.goto('/?motion=0')
  await ready(page)
  await expect(page.getByTestId('lore-rail')).toBeVisible()
  const sessionRows = page.locator('[data-testid="rail-item"][data-kind="sessions"]')
  await expect(sessionRows).toHaveCount(0)
  await page.getByTestId('rail-chip-sessions').click()
  expect(await sessionRows.count()).toBeGreaterThan(0)
  // Hover the first rail item that has anchored packages.
  const items = page.getByTestId('rail-item')
  const n = await items.count()
  let lit = 0
  for (let i = 0; i < n && lit === 0; i++) {
    await items.nth(i).hover()
    lit = await count(page, 'node.lore-hot')
  }
  expect(lit).toBeGreaterThan(0)
})

test('anchors are deterministic across reloads (overview and focus)', async ({ page }) => {
  await page.goto('/?motion=0')
  await ready(page)
  const ov1 = await anchors(page, 'node[kind = "package"]')
  const pkg = await biggestPackage(page)
  await page.goto(`/?motion=0&pkg=${encodeURIComponent(pkg.label)}`)
  await ready(page)
  const fc1 = await anchors(page, 'node[kind = "symbol"]')
  await page.goto('/?motion=0')
  await ready(page)
  const ov2 = await anchors(page, 'node[kind = "package"]')
  await page.goto(`/?motion=0&pkg=${encodeURIComponent(pkg.label)}`)
  await ready(page)
  const fc2 = await anchors(page, 'node[kind = "symbol"]')
  expect(ov2).toEqual(ov1)
  expect(fc2).toEqual(fc1)
})

test('deep link with motion: anchors settle correctly (transition-race regression)', async ({ page }) => {
  await page.goto('/?motion=0')
  await ready(page)
  const pkg = await biggestPackage(page)
  await page.goto(`/?pkg=${encodeURIComponent(pkg.label)}`)  // NO motion=0 — real motion path
  await ready(page)
  await page.waitForTimeout(500)
  const offAnchor = await page.evaluate(() => {
    let worst = 0
    window.__cy.$('node[kind = "symbol"]').forEach((n: any) => {
      const p = n.position()
      const d = Math.hypot(p.x - n.data('ax'), p.y - n.data('ay'))
      if (d > worst) worst = d
    })
    return worst
  })
  // Rendered positions may oscillate around anchors by ≤2.5px (+ small epsilon);
  // a corrupted anchor shows up as a large offset.
  expect(offAnchor).toBeLessThan(4)
})

test('motion: anchors still, rendered positions drift', async ({ page }) => {
  await page.goto('/')
  await ready(page)
  const snap = () =>
    page.evaluate(() => {
      const a: Record<string, { ax: number; ay: number; x: number; y: number }> = {}
      window.__cy.$('node[kind = "package"]').forEach((n: any) => {
        const p = n.position()
        a[n.id()] = { ax: n.data('ax'), ay: n.data('ay'), x: p.x, y: p.y }
      })
      return a
    })
  const s1 = await snap()
  await page.waitForTimeout(600)
  const s2 = await snap()
  const ids = Object.keys(s1)
  expect(ids.length).toBeGreaterThan(0)
  for (const id of ids) {
    expect(s2[id].ax).toBe(s1[id].ax)
    expect(s2[id].ay).toBe(s1[id].ay)
  }
  const moved = ids.some((id) => s1[id].x !== s2[id].x || s1[id].y !== s2[id].y)
  expect(moved).toBe(true)
})
