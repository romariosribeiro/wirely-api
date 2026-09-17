// Generates repository screenshots against an isolated temporary Wirely database.
// Example:
// make build
// WIRELY_PLAYWRIGHT_MODULE=/path/to/playwright/index.mjs node web/e2e/screenshots.mjs
import assert from 'node:assert/strict'
import { mkdir, mkdtemp, rm } from 'node:fs/promises'
import { tmpdir } from 'node:os'
import { join, resolve } from 'node:path'
import { createServer } from 'node:net'
import { spawn } from 'node:child_process'

const { chromium } = await import(process.env.WIRELY_PLAYWRIGHT_MODULE || 'playwright')
const root = resolve(import.meta.dirname, '../..')
const output = join(root, 'docs/images')
const data = await mkdtemp(join(tmpdir(), 'wirely-screenshots-'))
const binary = resolve(root, process.env.WIRELY_SCREENSHOT_BINARY || 'bin/wirely')

const socket = createServer()
await new Promise((done) => socket.listen(0, '127.0.0.1', done))
const port = socket.address().port
await new Promise((done) => socket.close(done))
const base = `http://127.0.0.1:${port}`

const app = spawn(binary, [], {
  cwd: root,
  env: { ...process.env, WIRELY_DATA_DIR: data, WIRELY_ADDRESS: `127.0.0.1:${port}` },
  stdio: ['ignore', 'pipe', 'pipe'],
})

let password = ''
let logs = ''
const readLog = (chunk) => {
  logs += chunk.toString()
  const json = logs.match(/"initial_password":"([^"]+)"/)
  const legacy = logs.match(/INITIAL ADMIN PASSWORD: (\S+)/)
  password ||= json?.[1] || legacy?.[1] || ''
}
app.stdout.on('data', readLog)
app.stderr.on('data', readLog)

let browser
try {
  for (let attempt = 0; attempt < 100; attempt += 1) {
    if (app.exitCode !== null) throw new Error(`Wirely exited before startup:\n${logs}`)
    if (password && await fetch(`${base}/api/health`).then((response) => response.ok).catch(() => false)) break
    await new Promise((done) => setTimeout(done, 100))
  }
  assert.ok(password, `Initial password was not found in the isolated server log:\n${logs}`)

  await mkdir(output, { recursive: true })
  browser = await chromium.launch({ headless: true })
  const page = await browser.newPage({ viewport: { width: 1440, height: 1000 }, deviceScaleFactor: 1 })
  await page.goto(base, { waitUntil: 'networkidle' })
  await page.screenshot({ path: join(output, 'wirely-login.png'), fullPage: true, animations: 'disabled' })

  const login = await page.request.post(`${base}/api/auth/login`, { data: { username: 'admin', password } })
  assert.equal(login.status(), 200, `Isolated login failed: ${await login.text()}`)
  for (const name of ['Atendimento Principal', 'Suporte Comercial']) {
    const created = await page.request.post(`${base}/api/instances`, { data: { name } })
    assert.equal(created.status(), 201, `Creating ${name} failed: ${await created.text()}`)
  }
  await page.goto(base, { waitUntil: 'networkidle' })
  await page.getByRole('heading', { name: 'Instâncias', exact: true }).waitFor()
  await page.screenshot({ path: join(output, 'wirely-dashboard.png'), fullPage: true, animations: 'disabled' })

  await page.getByRole('button', { name: 'Documentação' }).click()
  await page.getByRole('dialog', { name: 'Documentação' }).waitFor()
  await page.screenshot({ path: join(output, 'wirely-documentation.png'), animations: 'disabled' })
} finally {
  await browser?.close()
  app.kill('SIGTERM')
  await rm(data, { recursive: true, force: true })
}
