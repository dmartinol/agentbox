#!/usr/bin/env node
// Captures docs/screenshots/flow.gif from the built GUI with deterministic API
// fixtures. Run `npm --prefix internal/gui/web run tour:capture` (which builds
// first). Requires Node >= 22.6, Google Chrome (or TOUR_CHROME=/path/to/chrome)
// and ffmpeg on PATH.

import { execFileSync } from 'node:child_process';
import { createServer } from 'node:http';
import { existsSync, mkdirSync, mkdtempSync, readFileSync, rmSync, statSync, writeFileSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { extname, join, resolve, dirname } from 'node:path';
import { fileURLToPath } from 'node:url';
import { chromium } from 'playwright-core';
import { mcps, overview, providers, runs, skillEntries, skillStatus } from '../tests/fixtures.ts';

const here = dirname(fileURLToPath(import.meta.url));
const webRoot = resolve(here, '..');
const repoRoot = resolve(webRoot, '../../..');
const distRoot = join(webRoot, 'dist');
const outFile = process.env.TOUR_OUT ? resolve(process.env.TOUR_OUT) : join(repoRoot, 'docs/screenshots/flow.gif');
const appWidth = 1440;
const appHeight = 768;
const captionHeight = 132;
const fps = 5;

if (!existsSync(join(distRoot, 'index.html'))) {
  console.error(`capture-tour: ${distRoot}/index.html is missing; run npm --prefix internal/gui/web run build first`);
  process.exit(1);
}

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
  if (process.env.TOUR_CHROME) return process.env.TOUR_CHROME;
  const bundled = chromium.executablePath();
  if (bundled && existsSync(bundled)) return bundled;
  const candidates = [
    '/Applications/Google Chrome.app/Contents/MacOS/Google Chrome',
    '/Applications/Chromium.app/Contents/MacOS/Chromium',
    '/usr/bin/google-chrome',
    '/usr/bin/chromium',
    '/usr/bin/chromium-browser'
  ];
  const found = candidates.find((candidate) => existsSync(candidate));
  if (!found) {
    console.error('capture-tour: no Chrome found; set TOUR_CHROME=/path/to/chrome');
    process.exit(1);
  }
  return found;
}

const liveRun = runs.find((run) => run.id === 'run-active');
const denial = {
  host: 'api.example.com',
  port: 443,
  binary: '/usr/local/bin/codex',
  observedBinary: '/usr/bin/curl',
  reason: 'No matching network policy rule',
  valid: true
};
const policyDiff = `@@ -9,6 +9,10 @@ network_policies:
     binaries: ["/usr/local/bin/codex"]
     rules:
       - allow: { method: GET, path: "/v1/**" }
+  - host: api.example.com
+    port: 443
+    binaries: ["/usr/local/bin/codex"]
+    source: run
   - host: github.com
     port: 443
     binaries: ["/usr/bin/git"]`;
const manifestDiff = `@@ -12,6 +12,9 @@ workers:
       network:
         hosts:
           - host: github.com
             port: 443
+          - host: api.example.com
+            port: 443
+            binaries: ["/usr/local/bin/codex"]`;
const fragmentDiff = `@@ -1,3 +1,7 @@
+# Added by agentbox run run-active
+host: api.example.com
+port: 443
+binaries: ["/usr/local/bin/codex"]
`;
const preview = {
  entry: { host: denial.host, port: denial.port, binaries: [denial.binary] },
  policyPath: '/policies/run-active.yaml',
  policyDiff,
  persistAvailable: true,
  manifestPath: '/repo/.agentbox/config.yaml',
  manifestDiff,
  fragmentPath: '/repo/.agentbox/policies/api.example.com.yaml',
  fragmentDiff
};
const applied = {
  ...preview,
  outcome: 'live-applied',
  rolledBack: false,
  runtimeHost: {
    host: denial.host,
    port: denial.port,
    binary: denial.binary,
    rule: 'exact',
    source: 'run',
    added_at: '2026-11-23T21:00:00Z'
  },
  persisted: false
};

const providersWithModes = providers.map((provider) => ({
  ...provider,
  mode: provider.name === 'openai' || provider.name === 'anthropic' ? 'standard' : 'gateway'
}));

const tourProjects = overview.projects.map((project) => ({
  ...project,
  checkout: { state: 'up-to-date', ahead: 0, behind: 0 }
}));

function apiResponse(method, pathname, searchParams) {
  const key = `${method} ${pathname}`;
  if (key === 'GET /api/overview') return { ...overview, projects: tourProjects };
  if (key === 'GET /api/runs') return runs;
  if (key === 'GET /api/mcps') return mcps;
  if (key === 'GET /api/skills') return skillEntries;
  if (key === 'GET /api/skills/status') return skillStatus;
  if (key === 'GET /api/doctor') return overview.doctor;
  if (key === 'GET /api/profiles') return overview.profiles;
  if (key === 'GET /api/providers') return providersWithModes;
  if (key === 'GET /api/images') return overview.images;
  if (key === 'GET /api/projects') return tourProjects;
  if (key === 'GET /api/gateways') return overview.gateways;
  if (key === 'GET /api/registry') return { reference: 'quay.io/example/openshell' };
  if (key === `GET /api/runs/${liveRun.id}/logs`) {
    return {
      logSource: 'screen',
      denials: [denial],
      logs: [
        '> Build the profile summary endpoint and run the tests.',
        '',
        '$ go test ./internal/app/...',
        'ok  \tgithub.com/example/demo/internal/app\t2.4s',
        '',
        '$ curl -sS https://api.example.com/v1/profile',
        'curl: (7) Failed to connect to api.example.com port 443',
        '',
        'Sandbox policy denied network access to api.example.com:443 for /usr/local/bin/codex.',
        'Retry after the host is added to the run policy.'
      ].join('\n')
    };
  }
  if (key === `GET /api/runs/${liveRun.id}/shells`) return ['shell-1'];
  if (key === `GET /api/runs/${liveRun.id}/policy/content`) {
    return { path: '/policies/run-active.yaml', content: 'version: 1\nnetwork_policies: []\n' };
  }
  if (key === `GET /api/runs/${liveRun.id}/diff`) return { diff: 'diff --git a/api/profile.go b/api/profile.go' };
  if (key === `POST /api/runs/${liveRun.id}/policy/denial/preview`) return preview;
  if (key === `POST /api/runs/${liveRun.id}/policy/denial`) return applied;
  if (key === `GET /api/providers/model-gateway/models` && searchParams.get('gateway') === 'openshell') {
    return ['model-x', 'model-y'];
  }
  return undefined;
}

async function installApiMocks(context) {
  await context.route('**/api/**', async (route) => {
    const request = route.request();
    const url = new URL(request.url());
    const body = apiResponse(request.method(), url.pathname, url.searchParams);
    if (body === undefined) {
      await route.fulfill({ status: 404, contentType: 'application/json', body: JSON.stringify({ error: `unmocked ${request.method()} ${url.pathname}` }) });
      return;
    }
    await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify(body) });
  });
}

function captionHtml(step, title, subtitle) {
  return `<!doctype html><meta charset="utf-8"><style>
  html,body{margin:0;width:${appWidth}px;height:${captionHeight}px;overflow:hidden}
  body{background:#0b1220;color:#f8fafc;font-family:-apple-system,BlinkMacSystemFont,"Segoe UI",Inter,system-ui,sans-serif;display:flex;align-items:center;justify-content:space-between;padding:0 40px;box-sizing:border-box;border-top:1px solid rgba(148,163,184,.25)}
  .left{display:flex;flex-direction:column;gap:7px;min-width:0}
  .kicker{font-size:12px;letter-spacing:.24em;color:#7dd3fc;font-weight:600;text-transform:uppercase}
  .title{font-size:27px;font-weight:650;letter-spacing:-.01em;line-height:1.1}
  .sub{font-size:15px;color:#94a3b8;line-height:1.2;max-width:1120px}
  .badge{flex:none;width:56px;height:56px;border-radius:999px;background:#2563eb;display:flex;align-items:center;justify-content:center;font-size:20px;font-weight:600}
  </style><div class="left"><div class="kicker">AgentBox / Quick tour</div><div class="title">${title}</div><div class="sub">${subtitle}</div></div><div class="badge">${step}</div>`;
}

function run(command, args) {
  execFileSync(command, args, { stdio: ['ignore', 'ignore', 'inherit'] });
}

async function main() {
  const work = mkdtempSync(join(tmpdir(), 'agentbox-tour-'));
  const frames = join(work, 'frames');
  mkdirSync(frames, { recursive: true });
  const { server, port } = await serveDist();
  const browser = await chromium.launch({ executablePath: chromePath(), headless: true, args: ['--force-color-profile=srgb'] });
  try {
    const context = await browser.newContext({ viewport: { width: appWidth, height: appHeight }, deviceScaleFactor: 1, colorScheme: 'light' });
    await installApiMocks(context);
    const app = await context.newPage();
    await app.goto(`http://127.0.0.1:${port}/`, { waitUntil: 'networkidle' });
    await app.getByText('active-run', { exact: true }).first().waitFor();
    const captionPage = await context.newPage();
    await captionPage.setViewportSize({ width: appWidth, height: captionHeight });

    const scenes = [];
    async function captureApp(file) {
      await app.waitForTimeout(150);
      await app.screenshot({ path: join(work, file), animations: 'disabled' });
    }
    async function scene(step, title, subtitle, appFile) {
      const capFile = join(work, `caption-${step}.png`);
      await captionPage.setContent(captionHtml(step, title, subtitle), { waitUntil: 'load' });
      await captionPage.screenshot({ path: capFile, animations: 'disabled' });
      const outFile = join(frames, `scene-${step}.png`);
      run('ffmpeg', ['-y', '-v', 'error', '-i', join(work, appFile), '-i', capFile, '-filter_complex', '[0:v][1:v]vstack=inputs=2', outFile]);
      scenes.push(outFile);
    }

    // 01 Fleet overview.
    await captureApp('app-01.png');
    await scene('01', 'Monitor the fleet', 'Every agent grouped by project, with live status, sync state, and branch.', 'app-01.png');

    // 02 Setup sheet, Profiles tab.
    await app.getByRole('button', { name: 'Setup' }).click();
    await app.getByRole('tab', { name: 'Profiles' }).click();
    await app.getByRole('tabpanel').getByText('codex-default', { exact: true }).first().waitFor();
    await captureApp('app-02.png');
    await scene('02', 'Configure agent profiles', 'Profiles, providers, images, gateways, MCP tools, and skills live in Setup.', 'app-02.png');

    // 03 Setup sheet, Projects tab (no Policies tab).
    await app.getByRole('tab', { name: 'Projects' }).click();
    await app.getByRole('tabpanel').getByText('demo', { exact: true }).first().waitFor();
    await captureApp('app-03.png');
    await scene('03', 'Register projects', 'Each project keeps isolated checkouts and a shared worker image ready for a run.', 'app-03.png');

    // 04 Fleet with the live agent selected.
    await app.keyboard.press('Escape');
    await app.getByRole('dialog').waitFor({ state: 'detached' });
    await app.getByText('active-run', { exact: true }).first().click();
    await app.getByRole('complementary', { name: 'Agent details' }).getByRole('button', { name: 'View logs' }).waitFor();
    await app.getByRole('button', { name: 'View logs' }).click();
    await app.getByText('Network request denied').waitFor();
    await app.evaluate(() => {
      const section = document.querySelector('section[aria-label="Network denials"]');
      const scroller = section?.parentElement;
      if (scroller) scroller.scrollTop = 0;
    });
    await app.waitForTimeout(150);
    await captureApp('app-04.png');
    await scene('04', 'Inspect a sandbox denial', 'Blocked network requests appear as structured events above the raw agent output.', 'app-04.png');

    // 05 Add-host dialog with the scoped diff.
    await app.getByRole('button', { name: `Add host ${denial.host}:${denial.port}` }).click();
    await app.getByText('Run policy diff').waitFor();
    await captureApp('app-05.png');
    await scene('05', 'Allow the host', 'Review the exact host, port, binary, and policy diff before changing the run.', 'app-05.png');

    // 06 Applied result.
    await app.getByRole('button', { name: 'Add host', exact: true }).click();
    await app.getByText('Network rule applied').waitFor();
    await captureApp('app-06.png');
    await scene('06', 'Rule applied', 'The agent can retry immediately; persist the rule when it should outlive this run.', 'app-06.png');

    const list = join(work, 'scenes.txt');
    writeFileSync(list, scenes.map((file) => `file '${file}'\nduration 1.0`).join('\n') + `\nfile '${scenes.at(-1)}'\n`);
    const palette = join(work, 'palette.png');
    run('ffmpeg', ['-y', '-v', 'error', '-f', 'concat', '-safe', '0', '-i', list, '-vf', `fps=${fps},palettegen=max_colors=96:stats_mode=diff`, palette]);
    run('ffmpeg', ['-y', '-v', 'error', '-f', 'concat', '-safe', '0', '-i', list, '-i', palette, '-lavfi', `fps=${fps}[x];[x][1:v]paletteuse=dither=bayer:bayer_scale=4:diff_mode=rectangle`, '-loop', '0', outFile]);
    console.log(`capture-tour: wrote ${outFile}`);
  } finally {
    await browser.close();
    server.close();
    if (!process.env.TOUR_KEEP_FRAMES) rmSync(work, { recursive: true, force: true });
    else console.log(`capture-tour: frames kept in ${work}`);
  }
}

main().catch((error) => {
  console.error(error);
  process.exit(1);
});
