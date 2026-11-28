#!/usr/bin/env node
// Renders Setup -> Projects in a real browser at 390px, 768px, and desktop
// widths, and fails when the surface clips, overlaps, loses an accessible name,
// or needs horizontal scrolling. Run `npm --prefix internal/gui/web run
// smoke:responsive`. Requires Node >= 22.6, a built dist, and a local
// Chrome/Chromium (or SMOKE_CHROME=/path/to/chrome). Without a browser it skips
// with exit 0 unless --require-browser is passed.

import { createServer } from 'node:http';
import { existsSync, mkdirSync, readFileSync, statSync } from 'node:fs';
import { extname, join, resolve, dirname } from 'node:path';
import { tmpdir } from 'node:os';
import { fileURLToPath } from 'node:url';
import { chromium } from 'playwright-core';
import { mcps, overview, providers, runs, skillEntries, skillStatus } from '../tests/fixtures.ts';

const here = dirname(fileURLToPath(import.meta.url));
const webRoot = resolve(here, '..');
const distRoot = join(webRoot, 'dist');
const requireBrowser = process.argv.includes('--require-browser');
const headed = process.argv.includes('--headed') || process.env.SMOKE_HEADED === '1';
const slowMo = Number(process.env.SMOKE_SLOWMO ?? 0) || 0;

const viewports = [
  { label: 'mobile 390x844', width: 390, height: 844 },
  { label: 'tablet 768x1024', width: 768, height: 1024 },
  { label: 'desktop 1440x900', width: 1440, height: 900 }
];

const mime = {
  '.html': 'text/html; charset=utf-8',
  '.js': 'text/javascript; charset=utf-8',
  '.css': 'text/css; charset=utf-8',
  '.svg': 'image/svg+xml',
  '.png': 'image/png',
  '.json': 'application/json',
  '.woff2': 'font/woff2',
  '.woff': 'font/woff'
};

function serveDist() {
  const server = createServer((req, res) => {
    const pathname = decodeURIComponent(new URL(req.url, 'http://127.0.0.1').pathname);
    const file = pathname === '/' ? join(distRoot, 'index.html') : join(distRoot, pathname);
    const target = file.startsWith(distRoot) && existsSync(file) && statSync(file).isFile() ? file : join(distRoot, 'index.html');
    res.writeHead(200, { 'Content-Type': mime[extname(target)] ?? 'application/octet-stream' });
    res.end(readFileSync(target));
  });
  return new Promise((ok) => server.listen(0, '127.0.0.1', () => ok({ server, port: server.address().port })));
}

function chromePath() {
  if (process.env.SMOKE_CHROME) return process.env.SMOKE_CHROME;
  try {
    const bundled = chromium.executablePath();
    if (bundled && existsSync(bundled)) return bundled;
  } catch {
    // playwright-core ships no browser; fall through to a local install.
  }
  const candidates = [
    '/Applications/Google Chrome.app/Contents/MacOS/Google Chrome',
    '/Applications/Chromium.app/Contents/MacOS/Chromium',
    '/usr/bin/google-chrome',
    '/usr/bin/chromium',
    '/usr/bin/chromium-browser'
  ];
  return candidates.find((candidate) => existsSync(candidate));
}

const providersWithModes = providers.map((provider) => ({
  ...provider,
  mode: provider.name === 'openai' || provider.name === 'anthropic' ? 'standard' : 'gateway'
}));

// The Projects table shows a checkout state derived from local refs. Pin one so
// the check asserts a real value instead of "unknown".
const smokeProjects = overview.projects.map((project) => ({
  ...project,
  checkout: { state: 'behind', ahead: 0, behind: 3 }
}));

function apiResponse(method, pathname) {
  const key = `${method} ${pathname}`;
  if (key === 'GET /api/overview') return { ...overview, projects: smokeProjects };
  if (key === 'GET /api/runs') return runs;
  if (key === 'GET /api/mcps') return mcps;
  if (key === 'GET /api/skills') return skillEntries;
  if (key === 'GET /api/skills/status') return skillStatus;
  if (key === 'GET /api/doctor') return overview.doctor;
  if (key === 'GET /api/profiles') return overview.profiles;
  if (key === 'GET /api/providers') return providersWithModes;
  if (key === 'GET /api/images') return overview.images;
  if (key === 'GET /api/projects') return smokeProjects;
  if (key === 'GET /api/gateways') return overview.gateways;
  if (key === 'GET /api/registry') return { reference: 'quay.io/example/openshell' };
  return undefined;
}

async function installApiMocks(context) {
  await context.route('**/api/**', async (route) => {
    const request = route.request();
    const url = new URL(request.url());
    const body = apiResponse(request.method(), url.pathname);
    if (body === undefined) {
      await route.fulfill({ status: 404, contentType: 'application/json', body: JSON.stringify({ error: `unmocked ${request.method()} ${url.pathname}` }) });
      return;
    }
    await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify(body) });
  });
}

function overlaps(a, b) {
  const width = Math.min(a.x + a.width, b.x + b.width) - Math.max(a.x, b.x);
  const height = Math.min(a.y + a.height, b.y + b.height) - Math.max(a.y, b.y);
  return width > 4 && height > 4;
}

async function box(locator, viewport, label) {
  const rect = await locator.boundingBox();
  if (!rect) throw new Error(`${label} has no layout box`);
  if (rect.x < -1 || rect.x + rect.width > viewport.width + 1) {
    throw new Error(`${label} extends outside the ${viewport.width}px viewport (x=${Math.round(rect.x)}, right=${Math.round(rect.x + rect.width)})`);
  }
  return rect;
}

async function captureFailure(page, viewport) {
  try {
    const dir = process.env.SMOKE_OUT ? resolve(process.env.SMOKE_OUT) : join(tmpdir(), 'agentbox-responsive-smoke');
    mkdirSync(dir, { recursive: true });
    const file = join(dir, `${viewport.label.replace(/[^a-z0-9]+/gi, '-')}.png`);
    await page.screenshot({ path: file, animations: 'disabled', fullPage: true });
    return file;
  } catch {
    return undefined;
  }
}

async function checkViewport(browser, origin, viewport) {
  const context = await browser.newContext({ viewport: { width: viewport.width, height: viewport.height }, colorScheme: 'light' });
  await installApiMocks(context);
  const page = await context.newPage();
  const pageErrors = [];
  page.on('pageerror', (error) => pageErrors.push(error.message));
  try {
    await page.goto(origin, { waitUntil: 'networkidle' });
    await page.getByRole('button', { name: 'Setup' }).click();
    await page.getByRole('tab', { name: 'Projects' }).click();
    const panel = page.getByRole('tabpanel');
    await panel.getByText('demo', { exact: true }).first().waitFor({ timeout: 15000 });

    const identity = panel.getByText('demo', { exact: true }).first();
    const checkout = panel.locator('td[data-label="Checkout"]').first();
    const primary = panel.getByRole('button', { name: 'Create project' }).first();

    for (const [label, locator] of [['project identity', identity], ['checkout state', checkout], ['primary action', primary]]) {
      if (!(await locator.isVisible())) throw new Error(`${label} is not visible`);
    }

    const identityBox = await box(identity, viewport, 'project identity');
    const checkoutBox = await box(checkout, viewport, 'checkout state');
    const primaryBox = await box(primary, viewport, 'primary action');

    if (overlaps(identityBox, primaryBox) || overlaps(checkoutBox, primaryBox)) {
      throw new Error('primary action overlaps project identity or checkout state');
    }

    const overflow = await page.evaluate(() => document.documentElement.scrollWidth - window.innerWidth);
    if (overflow > 1) throw new Error(`page needs ${overflow}px of horizontal scrolling`);

    const buttons = panel.getByRole('button');
    const count = await buttons.count();
    for (let index = 0; index < count; index += 1) {
      const name = await buttons.nth(index).evaluate((element) => (element.getAttribute('aria-label') ?? element.textContent ?? '').trim());
      if (!name) throw new Error(`button ${index} has no accessible name`);
    }

    if (pageErrors.length) throw new Error(`page errors: ${pageErrors.join('; ')}`);
  } catch (error) {
    const shot = await captureFailure(page, viewport);
    throw new Error(`${error.message}${shot ? ` (screenshot: ${shot})` : ''}`);
  } finally {
    await context.close();
  }
}

async function main() {
  if (!existsSync(join(distRoot, 'index.html'))) {
    console.error(`responsive-smoke: ${distRoot}/index.html is missing; run npm --prefix internal/gui/web run build first`);
    process.exit(1);
  }
  const executablePath = chromePath();
  if (!executablePath) {
    const message = 'responsive-smoke: no Chrome or Chromium found; set SMOKE_CHROME=/path/to/chrome to run this check';
    if (requireBrowser) {
      console.error(message);
      process.exit(1);
    }
    console.log(`${message} (skipped)`);
    return;
  }
  const { server, port } = await serveDist();
  const browser = await chromium.launch({ executablePath, headless: !headed, slowMo, args: ['--force-color-profile=srgb'] });
  const failures = [];
  try {
    for (const viewport of viewports) {
      try {
        await checkViewport(browser, `http://127.0.0.1:${port}/`, viewport);
        console.log(`responsive-smoke: PASS ${viewport.label}`);
      } catch (error) {
        failures.push(`${viewport.label}: ${error.message}`);
        console.error(`responsive-smoke: FAIL ${viewport.label}: ${error.message}`);
      }
    }
  } finally {
    await browser.close();
    server.close();
  }
  if (failures.length) {
    console.error(`responsive-smoke: ${failures.length} of ${viewports.length} viewports failed`);
    process.exit(1);
  }
  console.log(`responsive-smoke: all ${viewports.length} viewports passed`);
}

await main();
