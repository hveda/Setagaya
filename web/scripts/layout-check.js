/**
 * Layout regression check for the operator SPA, at real viewport widths.
 *
 * Exists because jsdom -- what `vitest run` uses -- computes no geometry, so
 * the unit suite cannot see a layout bug. It missed a real one: the mobile
 * nav drawer was hidden with `invisible` while left in normal document flow,
 * so it still reserved its full height. <nav> measured 370px against
 * h-16's 64px and <main> started at y=370 on every page -- a ~306px band of
 * nothing above all content, about 46% of a 390x664 viewport. Nothing in the
 * repo could have caught that; this script can.
 *
 * Usage (from web/):
 *   bun run layout-check                       # against LAYOUT_CHECK_URL or the default
 *   LAYOUT_CHECK_URL=https://honryu.pve.heri.life bun run layout-check
 *   bun run layout-check -- --screenshots      # also write PNGs to .layout-check/
 *
 * Needs a Chromium binary. Playwright's cache is found automatically; set
 * CHROMIUM_PATH to point elsewhere. Provision with:
 *   bunx playwright install chromium
 *
 * Exits non-zero on the first failed assertion, so it works as a gate.
 */
import { chromium, devices } from 'playwright-core';
import { existsSync, mkdirSync, readdirSync } from 'node:fs';
import { homedir } from 'node:os';
import { join } from 'node:path';

const BASE_URL = (process.env.LAYOUT_CHECK_URL || 'http://localhost:4173').replace(/\/$/, '');
const WANT_SCREENSHOTS = process.argv.includes('--screenshots');
const SHOT_DIR = '.layout-check';

/** Routes the nav exposes -- every one carries the shared DashboardLayout. */
const ROUTES = ['/reports', '/reservations', '/status', '/campaigns', '/clusters'];

/**
 * Widths worth checking: below Tailwind's `md` (768px) the drawer is the only
 * nav, at and above it the inline links are and the drawer is `md:hidden`.
 * Both sides of that boundary can regress independently.
 */
const VIEWPORTS = [
  { name: 'mobile-390', width: 390, height: 664, isMobile: true },
  { name: 'tablet-768', width: 768, height: 1024, isMobile: false },
  { name: 'desktop-1280', width: 1280, height: 800, isMobile: false },
];

/**
 * The nav is h-16 (64px) plus a 1px bottom border. Allow a little slack for
 * sub-pixel rounding, but nowhere near enough to let a reserved drawer band
 * (hundreds of px) slip through.
 */
const NAV_HEIGHT_MAX = 72;

function findChromium() {
  if (process.env.CHROMIUM_PATH) {
    return process.env.CHROMIUM_PATH;
  }
  const cache = join(homedir(), '.cache', 'ms-playwright');
  if (!existsSync(cache)) {
    return null;
  }
  // Layouts differ across playwright versions and platforms.
  const candidates = ['chrome-linux64/chrome', 'chrome-linux/chrome', 'chrome-mac/Chromium.app/Contents/MacOS/Chromium'];
  for (const dir of readdirSync(cache).filter((d) => d.startsWith('chromium-'))) {
    for (const rel of candidates) {
      const full = join(cache, dir, rel);
      if (existsSync(full)) {
        return full;
      }
    }
  }
  return null;
}

const failures = [];
function check(label, ok, detail) {
  if (ok) {
    console.log(`  ok   ${label}`);
  } else {
    console.log(`  FAIL ${label}${detail ? ` -- ${detail}` : ''}`);
    failures.push(`${label}${detail ? `: ${detail}` : ''}`);
  }
}

/** Geometry the layout invariants are asserted against. */
function readGeometry() {
  const nav = document.querySelector('nav');
  const main = document.querySelector('main');
  const overflowing = [];
  document.querySelectorAll('*').forEach((el) => {
    const box = el.getBoundingClientRect();
    if (box.width > window.innerWidth + 1) {
      overflowing.push(`${el.tagName}.${(el.className || '').toString().trim().split(/\s+/)[0] || '(no class)'}=${Math.round(box.width)}px`);
    }
  });
  return {
    navHeight: nav ? Math.round(nav.getBoundingClientRect().height) : null,
    mainTop: main ? Math.round(main.getBoundingClientRect().top) : null,
    scrollWidth: document.documentElement.scrollWidth,
    innerWidth: window.innerWidth,
    overflowing: overflowing.slice(0, 5),
  };
}

/** Drawer state, for the mobile-only open/close assertions. */
function readDrawer() {
  const drawer = document.querySelector('.mobile-menu');
  if (!drawer) {
    return null;
  }
  const style = getComputedStyle(drawer);
  return {
    position: style.position,
    visibility: style.visibility,
    opacity: style.opacity,
    linkCount: drawer.querySelectorAll('a').length,
    mainTop: Math.round(document.querySelector('main').getBoundingClientRect().top),
  };
}

const executablePath = findChromium();
if (!executablePath) {
  console.error(
    'No Chromium found. Set CHROMIUM_PATH, or provision one with:\n  bunx playwright install chromium'
  );
  process.exit(2);
}

if (WANT_SCREENSHOTS) {
  mkdirSync(SHOT_DIR, { recursive: true });
}

console.log(`layout-check against ${BASE_URL}`);
console.log(`chromium: ${executablePath}\n`);

const browser = await chromium.launch({ executablePath, args: ['--ignore-certificate-errors'] });

try {
  for (const viewport of VIEWPORTS) {
    console.log(`${viewport.name} (${viewport.width}x${viewport.height})`);
    const context = await browser.newContext({
      viewport: { width: viewport.width, height: viewport.height },
      ...(viewport.isMobile ? devices['iPhone 13'] : {}),
      // Re-assert the width: the device descriptor carries its own viewport.
      ...(viewport.isMobile ? { viewport: { width: viewport.width, height: viewport.height } } : {}),
      ignoreHTTPSErrors: true,
    });
    const page = await context.newPage();

    for (const route of ROUTES) {
      let geo;
      try {
        await page.goto(BASE_URL + route, { waitUntil: 'networkidle', timeout: 20000 });
        await page.waitForSelector('nav', { timeout: 10000 });
        geo = await page.evaluate(readGeometry);
      } catch (err) {
        check(`${route} loads`, false, err.message.split('\n')[0]);
        continue;
      }

      // The bug this script exists for: a drawer left in flow inflates the
      // nav and pushes main down by its full height.
      check(
        `${route} nav height <= ${NAV_HEIGHT_MAX}px`,
        geo.navHeight !== null && geo.navHeight <= NAV_HEIGHT_MAX,
        `got ${geo.navHeight}px`
      );
      check(
        `${route} main sits directly under nav`,
        geo.mainTop !== null && Math.abs(geo.mainTop - geo.navHeight) <= 2,
        `main top ${geo.mainTop}px vs nav height ${geo.navHeight}px`
      );
      check(
        `${route} no horizontal overflow`,
        geo.scrollWidth <= geo.innerWidth + 1,
        geo.overflowing.length ? geo.overflowing.join(', ') : `scrollWidth ${geo.scrollWidth} > ${geo.innerWidth}`
      );

      if (WANT_SCREENSHOTS) {
        await page.screenshot({
          path: join(SHOT_DIR, `${viewport.name}${route.replace(/\//g, '-')}.png`),
          fullPage: true,
        });
      }
    }

    // Drawer behaviour is mobile-only: at md+ it is `md:hidden` and the
    // inline links take over, so there is no burger button to click.
    if (viewport.isMobile) {
      await page.goto(BASE_URL + ROUTES[0], { waitUntil: 'networkidle', timeout: 20000 });

      // A missing marker is itself a failure -- an older build, or a refactor
      // that dropped the class the click-outside handler matches on -- but it
      // has to report as one rather than throwing an uncaught timeout that
      // hides every remaining assertion behind a stack trace.
      const hasBurger = await page
        .waitForSelector('.mobile-menu-button', { timeout: 10000 })
        .then(() => true)
        .catch(() => false);
      check('burger button carries its .mobile-menu-button marker', hasBurger);

      if (hasBurger) {
        const closed = await page.evaluate(readDrawer);
        // The root invariant. Absolute in BOTH states is what keeps a closed
        // drawer from reserving height; asserting only the open state would
        // have passed against the original bug.
        check('drawer is absolutely positioned while closed', closed?.position === 'absolute', `got ${closed?.position}`);
        check('drawer is hidden while closed', closed?.visibility === 'hidden', `got ${closed?.visibility}`);

        await page.click('.mobile-menu-button');
        await page.waitForTimeout(500);
        const open = await page.evaluate(readDrawer);
        check('drawer becomes visible when opened', open?.visibility === 'visible', `got ${open?.visibility}`);
        check('drawer is still absolutely positioned when open', open?.position === 'absolute', `got ${open?.position}`);
        check(`drawer shows all ${ROUTES.length} nav links`, open?.linkCount === ROUTES.length, `got ${open?.linkCount}`);
        // Overlay, not push: opening must not move the page underneath.
        check(
          'opening the drawer does not shift main',
          closed != null && open != null && closed.mainTop === open.mainTop,
          `main moved ${closed?.mainTop} -> ${open?.mainTop}`
        );

        if (WANT_SCREENSHOTS) {
          await page.screenshot({ path: join(SHOT_DIR, `${viewport.name}-drawer-open.png`) });
        }

        // Tapping the page outside the drawer closes it -- the only natural
        // dismissal now that the drawer overlays content instead of moving it.
        await page.mouse.click(Math.round(viewport.width / 2), viewport.height - 80);
        await page.waitForTimeout(500);
        const reclosed = await page.evaluate(readDrawer);
        check('tapping outside closes the drawer', reclosed?.visibility === 'hidden', `got ${reclosed?.visibility}`);
      }
    }

    await context.close();
    console.log('');
  }
} finally {
  await browser.close();
}

if (failures.length > 0) {
  console.error(`layout-check FAILED (${failures.length}):`);
  failures.forEach((f) => console.error(`  - ${f}`));
  process.exit(1);
}
console.log('layout-check passed');
