// Run with WIRELY_PLAYWRIGHT_MODULE=/path/to/playwright/index.mjs node web/e2e/manage.mjs
// Uses an isolated backend and database; never connects a real WhatsApp account.
import assert from 'node:assert/strict'
import { spawn } from 'node:child_process'
import { mkdtemp, rm } from 'node:fs/promises'
import { tmpdir } from 'node:os'
import { join, resolve } from 'node:path'
import { createServer } from 'node:net'
const { chromium } = await import(process.env.WIRELY_PLAYWRIGHT_MODULE || 'playwright')

const directory = await mkdtemp(join(tmpdir(), 'wirely-manage-e2e-'))
const socket = createServer()
await new Promise((resolve) => socket.listen(0, '127.0.0.1', resolve))
const port = socket.address().port
await new Promise((resolve) => socket.close(resolve))
const base = `http://127.0.0.1:${port}`
const app = spawn(resolve('bin/wirely-preview'), [], {
 env: { ...process.env, WIRELY_DATA_DIR: directory, WIRELY_ADDRESS: `127.0.0.1:${port}` },
 stdio: ['ignore', 'ignore', 'pipe'],
})
let password = ''
let pendingLog = ''
app.stderr.on('data', (chunk) => {
 pendingLog += chunk.toString()
 const found = pendingLog.match(/INITIAL ADMIN PASSWORD: (\S+)/)
 if (found) { password = found[1]; pendingLog = '' }
})
let browser
try {
 for (let i = 0; i < 100; i++) {
  if (app.exitCode !== null) throw new Error('Isolated backend exited before becoming ready')
  if (password && await fetch(base + '/api/v1/health').then((r) => r.ok).catch(() => false)) break
  await new Promise((resolve) => setTimeout(resolve, 100))
 }
 assert.ok(password, 'initial credentials missing')
 browser = await chromium.launch({ headless: true })
 const page = await browser.newPage({ viewport: { width: 1440, height: 1050 } })
 const errors = []
 page.on('pageerror', (error) => errors.push(error.message))
 page.on('console', (message) => { if (message.type() === 'error' && !(message.location().url === base + '/api/v1/auth/me' && message.text().includes('401'))) errors.push(message.text()) })
 page.on('dialog', (dialog) => dialog.accept())
 await page.goto(base)
 await page.getByLabel('Senha', { exact: true }).fill(password)
 await page.getByRole('button', { name: 'Entrar', exact: true }).click()
 await page.getByLabel('Nome da instância').fill('Teste gerenciamento')
 await page.getByRole('button', { name: 'Nova instância' }).click()
 const modal = page.getByRole('dialog')
 await modal.waitFor()
 const initialToken = await modal.getByRole('textbox', { name: 'Token da API', exact: true }).inputValue()
 assert.ok(initialToken.startsWith('wly_'))
 await modal.getByRole('switch', { name: 'Webhook ativo' }).check()
 await modal.getByLabel('URL do webhook', { exact: true }).fill('https://example.com/events')
 await modal.getByRole('button', { name: 'Limpar seleção' }).click()
 await modal.getByRole('checkbox', { name: 'Mensagens · todos os eventos' }).check()
 await modal.getByRole('checkbox', { name: 'Status do WhatsApp' }).check()
 await modal.getByRole('button', { name: 'Salvar webhook', exact: true }).click()
 await modal.getByText('Webhook salvo com os eventos selecionados.').waitFor()
 const { data } = await page.request.get(base + '/api/v1/instances').then((r) => r.json())
 const endpoint = base + '/api/v1/instances/' + data[0].id
 let config = await page.request.get(endpoint + '/webhook').then((r) => r.json())
 assert.deepEqual(config.events.sort(), ['messages', 'status'])
 assert.equal(config.enabled, true)
 assert.equal(config.secret, undefined)
 await modal.getByRole('button', { name: 'Fechar', exact: true }).click()

 // Metrics use the real authenticated endpoint and remain usable on desktop and mobile.
 const metricsAPI = await page.request.get(base + '/api/v1/metrics?range=24h')
 assert.equal(metricsAPI.status(), 200)
 const metricsPayload = await metricsAPI.json()
 assert.equal(metricsPayload.range, '24h')
 assert.equal(metricsPayload.instances.total, 1)
 assert.equal(metricsPayload.byInstance[0].name, 'Teste gerenciamento')
 await page.getByRole('button', { name: 'Métricas', exact: true }).click()
 await modal.getByRole('heading', { name: 'Métricas operacionais', exact: true }).waitFor()
 await modal.getByText('Teste gerenciamento', { exact: true }).waitFor()
 const sevenDayResponse = page.waitForResponse((response) => response.url().includes('/api/v1/metrics?range=7d') && response.status() === 200)
 await modal.getByRole('button', { name: '7 dias', exact: true }).click()
 await sevenDayResponse
 await page.screenshot({ path: '/tmp/wirely-metrics-desktop.png', fullPage: true, animations: 'disabled' })
 await page.setViewportSize({ width: 390, height: 844 })
 assert.equal(await modal.evaluate((el) => el.scrollWidth <= el.clientWidth), true, 'metrics modal overflows horizontally')
 await page.screenshot({ path: '/tmp/wirely-metrics-mobile.png', fullPage: true, animations: 'disabled' })
 await modal.getByRole('button', { name: 'Fechar', exact: true }).click()
 await page.setViewportSize({ width: 1440, height: 1050 })

 // Team management uses real users, sessions and role middleware from the isolated backend.
 await page.getByRole('button', { name: 'Equipe', exact: true }).click()
 await modal.getByRole('heading', { name: 'Equipe', exact: true }).waitFor()
 await modal.getByLabel('Novo usuário', { exact: true }).fill('operator.browser')
 await modal.getByLabel('Senha inicial').fill('operator-browser-123')
 await modal.getByLabel('Papel do novo usuário').selectOption('operator')
 await modal.getByRole('button', { name: 'Adicionar', exact: true }).click()
 await modal.getByText('Usuário criado e pronto para acessar o painel.').waitFor()
 let operatorRow = modal.locator('.userRow', { hasText: 'operator.browser' })
 await operatorRow.waitFor()
 assert.equal(await operatorRow.getByLabel('Acesso ativo de operator.browser').isChecked(), true)

 const operatorContext = await browser.newContext()
 const operatorLogin = await operatorContext.request.post(base + '/api/auth/login', {
  data: { username: 'operator.browser', password: 'operator-browser-123' },
 })
 assert.equal(operatorLogin.status(), 200)
 assert.equal((await operatorContext.request.get(base + '/api/v1/instances')).status(), 200)
 assert.equal((await operatorContext.request.post(base + '/api/v1/instances', { data: { name: 'Negada' } })).status(), 403)
 assert.equal((await operatorContext.request.get(base + '/api/v1/users')).status(), 403)
 const operatorPage = await operatorContext.newPage({ viewport: { width: 1280, height: 800 } })
 await operatorPage.goto(base)
 await operatorPage.getByRole('heading', { name: 'Instâncias', exact: true }).waitFor()
 assert.equal(await operatorPage.getByRole('button', { name: 'Equipe', exact: true }).count(), 0)
 assert.equal(await operatorPage.getByRole('button', { name: 'Nova instância', exact: true }).count(), 0)
 assert.equal(await operatorPage.getByRole('button', { name: 'Gerenciar', exact: true }).count(), 0)

 await operatorRow.getByLabel('Papel de operator.browser').selectOption('viewer')
 await operatorRow.getByRole('button', { name: 'Salvar', exact: true }).click()
 await modal.getByText('Permissões de operator.browser atualizadas.').waitFor()
 assert.equal((await operatorContext.request.get(base + '/api/v1/auth/me')).status(), 401)
 operatorRow = modal.locator('.userRow', { hasText: 'operator.browser' })
 await operatorRow.getByRole('button', { name: 'Redefinir senha' }).click()
 await modal.getByLabel('Nova senha de operator.browser').fill('operator-reset-456')
 await modal.getByRole('button', { name: 'Salvar nova senha' }).click()
 await modal.getByText('Senha de operator.browser redefinida.').waitFor()
 await page.screenshot({ path: '/tmp/wirely-team-desktop.png', fullPage: true, animations: 'disabled' })
 await page.setViewportSize({ width: 390, height: 844 })
 assert.equal(await modal.evaluate((el) => el.scrollWidth <= el.clientWidth), true, 'team modal overflows horizontally')
 await page.screenshot({ path: '/tmp/wirely-team-mobile.png', fullPage: true, animations: 'disabled' })
 await page.setViewportSize({ width: 1440, height: 1050 })
 operatorRow = modal.locator('.userRow', { hasText: 'operator.browser' })
 await operatorRow.getByRole('button', { name: 'Excluir' }).click()
 await modal.getByText('Usuário operator.browser removido.').waitFor()
 await modal.locator('.userRow', { hasText: 'operator.browser' }).waitFor({ state: 'detached' })
 await operatorContext.close()
 await modal.getByRole('button', { name: 'Fechar', exact: true }).click()

 const openapi = await page.request.get(base + '/openapi.json')
 assert.equal(openapi.status(), 200)
 assert.equal(openapi.headers()['content-type'], 'application/json; charset=utf-8')
 const specification = await openapi.json()
 assert.equal(specification.openapi, '3.1.0')
 assert.ok(specification.paths['/api/send/media'])
 assert.equal(specification.paths['/api/send/image'], undefined)
 assert.ok(specification.paths['/api/queue/media'])
 assert.ok(specification.paths['/api/queue/text'])
 assert.ok(specification.paths['/api/queue/{jobID}/retry'])
 assert.ok(specification.paths['/api/v1/users'])
 assert.ok(specification.paths['/api/v1/metrics'])
 assert.ok(specification.paths['/api/v1/users/{userID}/password'])
 assert.ok(specification.components.schemas.MessageJob)
 assert.ok(specification.components.schemas.User)
 assert.ok(specification.components.schemas.MetricsSnapshot)
 assert.ok(specification.components.parameters.IdempotencyKey)
 assert.equal(specification.components.securitySchemes.BearerAuth.scheme, 'bearer')
 assert.equal(specification.components.securitySchemes.CookieAuth.name, 'wirely_session')
 await page.getByRole('button', { name: 'Documentação', exact: true }).click()
 await modal.getByRole('heading', { name: 'Documentação', exact: true }).waitFor()
 const docsCode = modal.locator('.docsCode:not(.docsQueueCode)')
 await page.waitForFunction((value) => document.querySelector('.docsCode')?.textContent?.includes(value), initialToken)
 assert.match(await docsCode.textContent(), /\/api\/send\/text/)
 assert.ok((await docsCode.textContent()).includes(base))
 await modal.getByRole('button', { name: /Imagem/ }).click()
 await modal.getByRole('tab', { name: 'Laravel' }).click()
 assert.match(await docsCode.textContent(), /Http::withToken/)
 assert.match(await docsCode.textContent(), /attach\('file'/)
 assert.match(await docsCode.textContent(), /'type' => 'image'/)
 await modal.getByRole('button', { name: /Áudio/ }).click()
 await modal.getByRole('tab', { name: 'Node.js' }).click()
 assert.match(await docsCode.textContent(), /new FormData/)
 assert.match(await docsCode.textContent(), /voice/)
 await modal.getByRole('button', { name: /Documento/ }).click()
 await modal.getByRole('tab', { name: 'Python' }).click()
 assert.match(await docsCode.textContent(), /requests\.post/)
 assert.match(await docsCode.textContent(), /invoice\.pdf/)
 await modal.getByRole('button', { name: /Figurinha/ }).click()
 assert.match(await docsCode.textContent(), /type.*sticker/)
 assert.match(await docsCode.textContent(), /sticker\.webp/)
 assert.equal(await modal.getByRole('link', { name: 'OpenAPI JSON' }).getAttribute('href'), '/openapi.json')
 const queueCode = modal.locator('.docsQueueCode')
 await modal.getByRole('heading', { name: 'Fila confiável' }).scrollIntoViewIfNeeded()
 assert.match(await queueCode.textContent(), /\/api\/queue\/text/)
 assert.match(await queueCode.textContent(), /\/api\/queue\/media/)
 assert.match(await queueCode.textContent(), /Idempotency-Key: pedido-123/)
 assert.ok((await queueCode.textContent()).includes(initialToken))
 await modal.getByRole('button', { name: 'Copiar código' }).click()
 await modal.getByRole('button', { name: /Copiado/ }).waitFor()
 await page.screenshot({ path: '/tmp/wirely-docs-desktop.png', fullPage: true, animations: 'disabled', mask: [docsCode, queueCode] })
 await page.setViewportSize({ width: 390, height: 844 })
 assert.equal(await modal.evaluate((el) => el.scrollWidth <= el.clientWidth), true, 'documentation overflows horizontally')
 await page.screenshot({ path: '/tmp/wirely-docs-mobile.png', fullPage: true, animations: 'disabled', mask: [docsCode] })
 await modal.getByRole('button', { name: 'Fechar', exact: true }).click()
 await page.setViewportSize({ width: 1440, height: 1050 })

 // Inbox data is simulated, while the browser exercises the real authenticated UI and routes.
 const inboxInstancesPattern = '**/api/v1/instances'
 const inboxChatsPattern = /\/api\/v1\/instances\/[^/]+\/chats\?/
 const inboxContactsPattern = /\/api\/v1\/instances\/[^/]+\/contacts\?/
 const inboxMessagesPattern = /\/api\/v1\/instances\/[^/]+\/chats\/[^/]+\/messages(?:\?|$)/
 const inboxReadPattern = /\/api\/v1\/instances\/[^/]+\/chats\/[^/]+\/read$/
 let inboxSent = false
 let inboxReadCalls = 0
 await page.route(inboxInstancesPattern, async (route) => {
  const response = await route.fetch()
  const result = await response.json()
  result.data = result.data.map((item) => ({ ...item, status: 'connected' }))
  await route.fulfill({ response, json: result })
 })
 await page.route(inboxChatsPattern, (route) => route.fulfill({ json: {
  data: [{ chat: '5511999999999@s.whatsapp.net', name: 'Cliente Inbox', isGroup: false, unreadCount: inboxReadCalls ? 0 : 1,
   lastMessage: { eventId: 'evt_inbox_1', messageId: 'msg_inbox_1', event: 'message.received', chat: '5511999999999@s.whatsapp.net', sender: '5511999999999@s.whatsapp.net', fromMe: false, isGroup: false, pushName: 'Cliente Inbox', type: 'text', text: 'Olá na central', timestamp: '2026-09-16T20:00:00Z', data: { text: 'Olá na central' } } }],
  page: 1, pageSize: 100, total: 1, totalPages: 1,
 } }))
 await page.route(inboxContactsPattern, (route) => route.fulfill({ json: {
  data: [{ jid: '5511999999999@s.whatsapp.net', name: 'Cliente Inbox', phone: '+5511999999999' }],
  page: 1, pageSize: 100, total: 1, totalPages: 1,
 } }))
 await page.route(inboxMessagesPattern, (route) => {
  if (route.request().method() === 'POST') {
   assert.deepEqual(route.request().postDataJSON(), { message: 'Resposta pela central' })
   inboxSent = true
   return route.fulfill({ status: 201, json: { id: 'msg_inbox_reply', recipient: '5511999999999@s.whatsapp.net', timestamp: '2026-09-16T20:02:00Z', type: 'text' } })
  }
  const reply = { eventId: 'evt_inbox_reply', messageId: 'msg_inbox_reply', event: 'message.sent', chat: '5511999999999@s.whatsapp.net', fromMe: true, isGroup: false, type: 'text', text: 'Resposta pela central', timestamp: '2026-09-16T20:02:00Z', data: { text: 'Resposta pela central' } }
  const received = { eventId: 'evt_inbox_1', messageId: 'msg_inbox_1', event: 'message.received', chat: '5511999999999@s.whatsapp.net', sender: '5511999999999@s.whatsapp.net', fromMe: false, isGroup: false, pushName: 'Cliente Inbox', type: 'text', text: 'Olá na central', timestamp: '2026-09-16T20:00:00Z', data: { text: 'Olá na central' } }
  return route.fulfill({ json: { data: inboxSent ? [reply, received] : [received], page: 1, pageSize: 50, total: inboxSent ? 2 : 1, totalPages: 1 } })
 })
 await page.route(inboxReadPattern, (route) => { inboxReadCalls++; return route.fulfill({ status: 204 }) })
 await page.reload()
 await page.getByRole('button', { name: 'Conversas', exact: true }).click()
 await modal.getByRole('heading', { name: 'Conversas · Teste gerenciamento' }).waitFor()
 await modal.getByText('Olá na central', { exact: true }).first().waitFor()
 await modal.locator('.inboxRow', { hasText: 'Cliente Inbox' }).click()
 await modal.getByRole('textbox', { name: 'Mensagem da conversa' }).fill('Resposta pela central')
 await modal.getByRole('button', { name: 'Enviar', exact: true }).click()
 await modal.locator('.messageBubble.outgoing', { hasText: 'Resposta pela central' }).waitFor()
 assert.equal(inboxSent, true)
 assert.ok(inboxReadCalls >= 1)
 await modal.getByRole('tab', { name: 'Contatos' }).click()
 await modal.getByText('Agenda do WhatsApp').waitFor()
 await modal.getByRole('tab', { name: 'Conversas' }).click()
 await page.screenshot({ path: '/tmp/wirely-inbox-desktop.png', fullPage: true, animations: 'disabled' })
 await page.setViewportSize({ width: 390, height: 844 })
 assert.equal(await modal.evaluate((el) => el.scrollWidth <= el.clientWidth), true, 'inbox modal overflows horizontally')
 await page.screenshot({ path: '/tmp/wirely-inbox-mobile.png', fullPage: true, animations: 'disabled' })
 await modal.getByRole('button', { name: 'Voltar para conversas' }).click()
 await modal.locator('.inboxSidebar').waitFor()
 await modal.getByRole('button', { name: 'Fechar conversas' }).click()
 await page.unroute(inboxInstancesPattern)
 await page.unroute(inboxChatsPattern)
 await page.unroute(inboxContactsPattern)
 await page.unroute(inboxMessagesPattern)
 await page.unroute(inboxReadPattern)
 await page.reload()
 await page.setViewportSize({ width: 1440, height: 1050 })

 // Activity history is simulated so the isolated test never needs a real WhatsApp connection or external webhook.
 let retryCalls = 0
 await page.route('**/api/v1/instances/*/events?*', (route) => route.fulfill({ json: {
  data: [{ id: 'evt_browser', instanceId: data[0].id, event: 'message.received', timestamp: '2026-09-16T20:00:00Z', data: { id: 'msg_browser', from: '5511999999999@s.whatsapp.net', text: 'Mensagem do histórico' } }],
  page: 1, pageSize: 20, total: 1, totalPages: 1,
 } }))
 await page.route('**/api/v1/instances/*/webhook-deliveries?*', (route) => route.fulfill({ json: {
  data: [{ id: 7, eventId: 'evt_browser', instanceId: data[0].id, event: 'message.received', url: 'https://example.com/events', attempt: 3, status: 'failed', httpStatus: 502, error: 'endpoint returned HTTP 502', durationMs: 42, manual: false, createdAt: '2026-09-16T20:00:01Z' }],
  page: 1, pageSize: 20, total: 1, totalPages: 1,
 } }))
 await page.route('**/api/v1/instances/*/webhook-deliveries/*/retry', (route) => {
  retryCalls++
  return route.fulfill({ json: { status: 'delivered', eventId: 'evt_browser' } })
 })
 let queueRetries = 0
 let queueCancels = 0
 await page.route('**/api/v1/instances/*/queue?*', (route) => route.fulfill({ json: {
  data: [
   { id: 'job_browser', instanceId: data[0].id, kind: 'text', recipient: '5511999999999', payload: { message: 'Mensagem agendada' }, status: queueCancels ? 'canceled' : 'queued', attempt: 0, maxAttempts: 5, scheduledAt: '2026-09-17T20:00:00Z', createdAt: '2026-09-16T20:00:00Z', updatedAt: '2026-09-16T20:00:00Z' },
   { id: 'job_failed', instanceId: data[0].id, kind: 'image', recipient: '5511888888888', payload: { caption: 'Imagem da fila', fileName: 'photo.png', fileSize: 2048 }, status: queueRetries ? 'queued' : 'failed', attempt: queueRetries ? 0 : 5, maxAttempts: 5, scheduledAt: '2026-09-16T19:00:00Z', lastError: queueRetries ? '' : 'instance is not connected', createdAt: '2026-09-16T19:00:00Z', updatedAt: '2026-09-16T20:00:00Z' },
  ], page: 1, pageSize: 20, total: 2, totalPages: 1,
 } }))
 await page.route('**/api/v1/instances/*/queue/*/retry', (route) => {
  queueRetries++
  return route.fulfill({ status: 202, json: { status: 'queued' } })
 })
 await page.route('**/api/v1/instances/*/queue/*', (route) => {
  queueCancels++
  return route.fulfill({ json: { status: 'canceled' } })
 })
 await page.getByRole('button', { name: 'Atividade', exact: true }).click()
 await modal.getByRole('heading', { name: 'Atividade · Teste gerenciamento' }).waitFor()
 await modal.getByText('Mensagem do histórico', { exact: true }).waitFor()
 await modal.getByLabel('Filtrar histórico').selectOption('messages')
 await modal.getByRole('tab', { name: 'Webhooks' }).click()
 await modal.getByText('Falha no webhook').waitFor()
 assert.equal(await modal.getByText('HTTP 502', { exact: true }).textContent(), 'HTTP 502')
 await modal.getByRole('button', { name: 'Reenviar webhook' }).click()
 await modal.getByText('Webhook entregue com sucesso.', { exact: false }).waitFor()
 assert.equal(retryCalls, 1)
 await modal.getByRole('tab', { name: 'Fila' }).click()
 await modal.getByText('Agendada · Texto').waitFor()
 await modal.getByRole('button', { name: 'Tentar novamente' }).click()
 await modal.getByText('Envio recolocado na fila.').waitFor()
 assert.equal(queueRetries, 1)
 await modal.getByRole('button', { name: 'Cancelar envio' }).first().click()
 await modal.getByText('Envio cancelado com segurança.').waitFor()
 assert.equal(queueCancels, 1)
 await page.screenshot({ path: '/tmp/wirely-activity-desktop.png', fullPage: true, animations: 'disabled' })
 await page.setViewportSize({ width: 390, height: 844 })
 assert.equal(await modal.evaluate((el) => el.scrollWidth <= el.clientWidth), true, 'activity modal overflows horizontally')
 await page.screenshot({ path: '/tmp/wirely-activity-mobile.png', fullPage: true, animations: 'disabled' })
 await modal.getByRole('button', { name: 'Fechar histórico' }).click()
 await page.unroute('**/api/v1/instances/*/events?*')
 await page.unroute('**/api/v1/instances/*/webhook-deliveries?*')
 await page.unroute('**/api/v1/instances/*/webhook-deliveries/*/retry')
 await page.unroute('**/api/v1/instances/*/queue?*')
 await page.unroute('**/api/v1/instances/*/queue/*/retry')
 await page.unroute('**/api/v1/instances/*/queue/*')
 await page.setViewportSize({ width: 1440, height: 1050 })

 await page.getByRole('button', { name: 'Gerenciar', exact: true }).click()
 await modal.getByRole('switch', { name: 'Webhook ativo' }).waitFor()
 await page.waitForFunction(() => !document.querySelector('dialog input[role="switch"]').disabled)
 await page.waitForFunction(() => Array.from(document.querySelectorAll('button')).some((el) => el.textContent === 'Gerar token' && !el.disabled))
 assert.equal(await modal.getByRole('textbox', { name: 'Token da API', exact: true }).inputValue(), initialToken)
 assert.equal(await modal.getByRole('checkbox', { name: 'Status do WhatsApp' }).isChecked(), true)
 await modal.getByRole('button', { name: 'Gerar token', exact: true }).click()
 await modal.getByText('Token gerado. Ele continuará disponível neste painel.').waitFor()
 const newToken = await modal.getByRole('textbox', { name: 'Token da API', exact: true }).inputValue()
 assert.notEqual(newToken, initialToken)
 const oldAuth = await page.request.post(base + '/api/send/text', { headers: { Authorization: 'Bearer ' + initialToken }, data: {} })
 assert.equal(oldAuth.status(), 401)
 await modal.getByRole('switch', { name: 'Webhook ativo' }).uncheck()
 await modal.getByRole('button', { name: 'Salvar webhook', exact: true }).click()
 await modal.getByText('Webhook desativado. Configurações preservadas.').waitFor()
 config = await page.request.get(endpoint + '/webhook').then((r) => r.json())
 assert.equal(config.enabled, false)
 assert.equal(config.url, 'https://example.com/events')
 assert.deepEqual(config.events.sort(), ['messages', 'status'])
 await modal.getByRole('button', { name: 'Fechar', exact: true }).click()
 await page.getByRole('button', { name: 'Gerenciar', exact: true }).click()
 await page.waitForFunction(() => !document.querySelector('dialog input[role="switch"]').disabled)
 await page.waitForFunction(() => Array.from(document.querySelectorAll('button')).some((el) => el.textContent === 'Gerar token' && !el.disabled))
 assert.equal(await modal.getByRole('textbox', { name: 'Token da API', exact: true }).inputValue(), newToken)
 await modal.getByRole('button', { name: 'Fechar', exact: true }).click()
 await page.reload()
 await page.getByRole('button', { name: 'Gerenciar', exact: true }).click()
 await page.waitForFunction(() => Array.from(document.querySelectorAll('button')).some((el) => el.textContent === 'Gerar token' && !el.disabled))
 assert.equal(await modal.getByRole('textbox', { name: 'Token da API', exact: true }).inputValue(), newToken)
 assert.equal(await modal.getByRole('switch', { name: 'Webhook ativo' }).isChecked(), false)
 // Screenshots must mask the visible token; the webhook signing secret stays one-time.
 await page.screenshot({ path: '/tmp/wirely-manage-desktop.png', fullPage: true, animations: 'disabled', mask: [page.getByRole('textbox', { name: 'Token da API', exact: true })] })
 await page.setViewportSize({ width: 390, height: 844 })
 assert.equal(await modal.evaluate((el) => el.scrollWidth <= el.clientWidth), true, 'modal overflows horizontally')
 await page.screenshot({ path: '/tmp/wirely-manage-mobile.png', fullPage: true, animations: 'disabled', mask: [page.getByRole('textbox', { name: 'Token da API', exact: true })] })
 await modal.getByRole('button', { name: 'Selecionar todos' }).click()
 assert.equal(await modal.getByRole('checkbox').count(), 5)
 for (const box of await modal.getByRole('checkbox').all()) assert.equal(await box.isChecked(), true)
 await modal.getByRole('button', { name: 'Limpar seleção' }).click()
 await modal.getByRole('switch', { name: 'Webhook ativo' }).check()
 await modal.getByRole('button', { name: 'Salvar webhook', exact: true }).click()
 await modal.getByText('Webhook salvo com os eventos selecionados.').waitFor()
 config = await page.request.get(endpoint + '/webhook').then((r) => r.json())
 assert.deepEqual(config.events, [])
 await modal.getByRole('button', { name: 'Fechar', exact: true }).click()
 // Legacy hash-only tokens are explained without silently rotating credentials.
 let automaticRotations = 0
 await page.route('**/api/v1/instances/*/token', (route) => {
  if (route.request().method() === 'GET') return route.fulfill({ json: { token: '', regenerationRequired: true } })
  automaticRotations++
  return route.continue()
 })
 await page.getByRole('button', { name: 'Gerenciar', exact: true }).click()
 await modal.getByText('Este token antigo foi salvo apenas como hash.', { exact: false }).waitFor()
 assert.equal(await modal.getByRole('textbox', { name: 'Token da API', exact: true }).inputValue(), '')
 assert.equal(await modal.getByRole('button', { name: 'Copiar', exact: true }).isDisabled(), true)
 assert.equal(automaticRotations, 0)
 await modal.getByRole('button', { name: 'Fechar', exact: true }).click()
 await page.unroute('**/api/v1/instances/*/token')
 // Pairing states are simulated: no real WhatsApp account is connected.
 await page.setViewportSize({ width: 1280, height: 800 })
 await page.screenshot({ path: '/tmp/wirely-dashboard-desktop.png', fullPage: true, animations: 'disabled', mask: [page.getByRole('textbox', { name: 'Token da API', exact: true })] })
 await page.setViewportSize({ width: 390, height: 844 })
 assert.equal(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth), true)
 await page.screenshot({ path: '/tmp/wirely-dashboard-mobile.png', fullPage: true, animations: 'disabled', mask: [page.getByRole('textbox', { name: 'Token da API', exact: true })] })
 await page.setViewportSize({ width: 1280, height: 800 })
 let pairingState = { status: 'connecting', qrAvailable: false }
 let qrRequests = 0
 let stateRequests = 0
 let pairingDisconnects = 0
 let invalidQR = false
 await page.route('**/api/v1/instances/*/connect', (route) => route.fulfill({ status: 204 }))
 await page.route('**/api/v1/instances/*/disconnect', (route) => {
  pairingDisconnects++
  return route.fulfill({ status: 204 })
 })
 await page.route('**/api/v1/instances/*/state', (route) => {
  stateRequests++
  return route.fulfill({ json: pairingState })
 })
 await page.route('**/api/v1/instances/*/qr?*', async (route) => {
  qrRequests++
  await new Promise((resolve) => setTimeout(resolve, 650))
  await route.fulfill({ contentType: 'image/svg+xml', body: invalidQR ? 'invalid image' : '<svg xmlns="http://www.w3.org/2000/svg" width="240" height="240"><rect width="240" height="240" fill="white"/><path d="M20 20h60v60H20zm140 0h60v60h-60zM20 160h60v60H20z" stroke="black" stroke-width="10" fill="none"/></svg>' })
 })
 await page.getByRole('button', { name: 'Conectar', exact: true }).click()
 await modal.getByRole('heading', { name: 'Preparando conexão' }).waitFor()
 const spinner = modal.locator('.spinner')
 const dimensions = await spinner.boundingBox()
 assert.ok(dimensions.width >= 40 && dimensions.height >= 40, 'loading indicator collapsed')
 assert.equal(await spinner.evaluate((el) => getComputedStyle(el).animationName), 'spin')
 await page.screenshot({ path: '/tmp/wirely-pairing-loading.png', fullPage: true, animations: 'disabled', mask: [page.getByRole('textbox', { name: 'Token da API', exact: true })] })
 pairingState = { status: 'qr', qrAvailable: true, qrExpiresAt: '2026-09-16T23:55:00Z' }
 await modal.getByRole('heading', { name: 'Carregando QR Code' }).waitFor()
 await modal.locator('.qrReady').waitFor()
 const firstRequests = qrRequests
 const firstStateRequests = stateRequests
 await page.waitForFunction(() => document.querySelector('.qrReady img')?.complete)
 // Wait for another status poll; unchanged QR must not download again.
 while (stateRequests <= firstStateRequests) await new Promise((resolve) => setTimeout(resolve, 100))
 assert.equal(qrRequests, firstRequests)
 pairingState = { status: 'connecting', qrAvailable: false }
 await modal.getByRole('heading', { name: 'Finalizando conexão' }).waitFor()
 assert.equal(await spinner.isVisible(), true)
 await page.setViewportSize({ width: 390, height: 844 })
 assert.equal(await modal.evaluate((el) => el.scrollWidth <= el.clientWidth), true)
 await page.screenshot({ path: '/tmp/wirely-pairing-mobile.png', fullPage: true, animations: 'disabled', mask: [page.getByRole('textbox', { name: 'Token da API', exact: true })] })
 pairingState = { status: 'connected', qrAvailable: false }
 await modal.getByRole('heading', { name: 'WhatsApp conectado' }).waitFor()
 assert.equal(await spinner.count(), 0)
 await page.screenshot({ path: '/tmp/wirely-pairing-success.png', fullPage: true, animations: 'disabled', mask: [page.getByRole('textbox', { name: 'Token da API', exact: true })] })
 await modal.getByRole('button', { name: 'Concluir', exact: true }).click()
 assert.equal(pairingDisconnects, 0, 'success must not disconnect')
 // Image errors have a retry, QR rotation reloads only the new code, and Escape cancels safely.
 pairingState = { status: 'qr', qrAvailable: true, qrExpiresAt: '2026-09-16T23:56:00Z' }
 invalidQR = true
 await page.getByRole('button', { name: 'Conectar', exact: true }).click()
 await modal.getByText('Não foi possível carregar o QR Code.').waitFor()
 invalidQR = false
 await modal.getByRole('button', { name: 'Tentar novamente' }).click()
 await modal.locator('.qrReady').waitFor()
 const beforeRotation = qrRequests
 pairingState = { ...pairingState, qrExpiresAt: '2026-09-16T23:57:00Z' }
 await modal.getByRole('heading', { name: 'Carregando QR Code' }).waitFor()
 await modal.locator('.qrReady').waitFor()
 assert.equal(qrRequests, beforeRotation + 1)
 await page.keyboard.press('Escape')
 await modal.waitFor({ state: 'detached' })
 assert.equal(pairingDisconnects, 1)
 pairingState = { status: 'connecting', qrAvailable: false }
 await page.emulateMedia({ reducedMotion: 'reduce' })
 await page.getByRole('button', { name: 'Conectar', exact: true }).click()
 await modal.getByRole('heading', { name: 'Preparando conexão' }).waitFor()
 assert.equal(await spinner.evaluate((el) => getComputedStyle(el).animationName), 'none')
 pairingState = { status: 'disconnected', qrAvailable: false }
 await modal.getByRole('heading', { name: 'Pareamento encerrado' }).waitFor()
 assert.equal(await spinner.count(), 0, 'expired pairing must not spin forever')
 await modal.getByRole('button', { name: 'Cancelar', exact: true }).click()
 await page.emulateMedia({ reducedMotion: 'no-preference' })
 await page.unroute('**/api/v1/instances/*/disconnect')
 let simulatedConnected = true
 let disconnectCalls = 0
 await page.route('**/api/v1/instances', async (route) => {
  const response = await route.fetch()
  const result = await response.json()
  result.data = result.data.map((item) => ({ ...item, status: simulatedConnected ? 'connected' : 'disconnected' }))
  await route.fulfill({ response, json: result })
 })
 await page.route('**/api/v1/instances/*/disconnect', async (route) => {
  assert.equal(route.request().url(), endpoint + '/disconnect')
  assert.equal(route.request().method(), 'POST')
  disconnectCalls++
  simulatedConnected = false
  await route.fulfill({ status: 204 })
 })
 await page.reload()
 await page.setViewportSize({ width: 1280, height: 900 })
 const sendCalls = []
 await page.route('**/api/send/*', async (route) => {
  const request = route.request()
  const endpointType = new URL(request.url()).pathname.split('/').at(-1)
  assert.equal(request.headers().authorization, 'Bearer ' + newToken)
  assert.equal(request.method(), 'POST')
  let messageType = endpointType
  if (endpointType === 'text') {
   assert.deepEqual(request.postDataJSON(), { recipient: '+55 11 99999-9999', message: 'Mensagem pelo playground' })
  } else {
   assert.equal(endpointType, 'media')
   const body = request.postDataBuffer().toString('latin1')
   assert.match(request.headers()['content-type'], /^multipart\/form-data; boundary=/)
   assert.ok(body.includes('name="recipient"'))
   assert.ok(body.includes('+55 11 99999-9999'))
   assert.ok(body.includes('name="file"'))
   const match = body.match(/name="type"\r\n\r\n([^\r]+)/)
   assert.ok(match, 'multipart media type missing')
   messageType = match[1]
  }
  sendCalls.push(messageType)
  await route.fulfill({ status: 201, contentType: 'application/json', body: JSON.stringify({
   id: 'msg_' + messageType, recipient: '+5511999999999', timestamp: '2026-09-16T20:00:00Z', type: messageType,
  }) })
 })
 await page.getByRole('button', { name: 'Testar API', exact: true }).click()
 await modal.getByRole('heading', { name: 'Testar envio · Teste gerenciamento' }).waitFor()
 await modal.getByLabel('Número com código do país').fill('+55 11 99999-9999')
 await modal.getByRole('textbox', { name: 'Mensagem', exact: true }).fill('Mensagem pelo playground')
 await modal.getByRole('button', { name: 'Enviar teste', exact: true }).click()
 await modal.getByText('Mensagem enviada').waitFor()
 assert.deepEqual(sendCalls, ['text'])

 await modal.getByRole('tab', { name: 'Imagem' }).click()
 await modal.getByLabel('Arquivo imagem').setInputFiles({ name: 'photo.png', mimeType: 'image/png', buffer: Buffer.from('89504e470d0a1a0a', 'hex') })
 await modal.getByLabel('Legenda opcional').fill('Imagem teste')
 await modal.getByRole('button', { name: 'Enviar teste', exact: true }).click()
 await modal.getByText('Mensagem enviada').waitFor()
 await modal.getByRole('tab', { name: 'Vídeo' }).click()
 await modal.getByLabel('Arquivo vídeo').setInputFiles({ name: 'clip.mp4', mimeType: 'video/mp4', buffer: Buffer.from('video test') })
 await modal.getByLabel('Legenda opcional').fill('Vídeo teste')
 await modal.getByRole('button', { name: 'Enviar teste', exact: true }).click()
 await modal.getByText('Mensagem enviada').waitFor()
 await modal.getByRole('tab', { name: 'Áudio' }).click()
 await modal.getByLabel('Arquivo áudio').setInputFiles({ name: 'voice.ogg', mimeType: 'audio/ogg', buffer: Buffer.from('OggS test') })
 await modal.getByLabel('Enviar como mensagem de voz').check()
 await modal.getByRole('button', { name: 'Enviar teste', exact: true }).click()
 await modal.getByText('Mensagem enviada').waitFor()
 await modal.getByRole('tab', { name: 'Documento' }).click()
 await modal.getByLabel('Arquivo documento').setInputFiles({ name: 'report.pdf', mimeType: 'application/pdf', buffer: Buffer.from('%PDF-1.7') })
 await modal.getByLabel('Legenda opcional').fill('Relatório teste')
 await modal.getByRole('button', { name: 'Enviar teste', exact: true }).click()
 await modal.getByText('Mensagem enviada').waitFor()
 await modal.getByRole('tab', { name: 'Figurinha' }).click()
 await modal.getByLabel('Arquivo figurinha').setInputFiles({ name: 'sticker.webp', mimeType: 'image/webp', buffer: Buffer.from('524946460c0000005745425056503820', 'hex') })
 await modal.getByRole('button', { name: 'Enviar teste', exact: true }).click()
 await modal.getByText('Mensagem enviada').waitFor()
 assert.deepEqual(sendCalls, ['text', 'image', 'video', 'audio', 'document', 'sticker'])
 await page.screenshot({ path: '/tmp/wirely-tester-desktop.png', fullPage: true, animations: 'disabled' })
 await page.setViewportSize({ width: 390, height: 844 })
 assert.equal(await modal.evaluate((el) => el.scrollWidth <= el.clientWidth), true, 'tester overflows horizontally')
 await page.screenshot({ path: '/tmp/wirely-tester-mobile.png', fullPage: true, animations: 'disabled' })
 await modal.getByRole('button', { name: 'Fechar', exact: true }).click()
 await page.setViewportSize({ width: 1280, height: 800 })
 await page.unroute('**/api/send/*')

 await page.getByRole('button', { name: 'Gerenciar', exact: true }).click()
 await modal.getByRole('button', { name: 'Desconectar', exact: true }).click()
 await modal.getByText('Instância desconectada.').waitFor()
 assert.equal(disconnectCalls, 1)
 assert.equal(await modal.getByRole('button', { name: 'Desconectar', exact: true }).isDisabled(), true)
 await modal.getByRole('button', { name: 'Excluir', exact: true }).click()
 await modal.waitFor({ state: 'detached' })
 assert.equal((await page.request.get(base + '/api/v1/instances').then((r) => r.json())).data.length, 0)
 assert.deepEqual(errors, [], 'browser console errors')
 console.log('PASS: metrics dashboard/API/ranges/responsive layout, team users/RBAC/session revocation, modal, persistent token across reopen/reload, explicit rotation, legacy token warning, playground text/image/video/audio/document/sticker via unified media endpoint, integrated OpenAPI docs and code examples, inbox chats/contacts/reply, activity filters, queue retry/cancel and webhook retry, saved event filters, disabling, empty selection, disconnect action (simulated connection), delete, desktop/mobile layout, pairing loading/QR/confirmation, QR caching/rotation/retry, cancellation, timeout, reduced motion.')
} finally {
 if (browser) await browser.close()
 app.kill('SIGTERM')
 if (app.exitCode === null) await new Promise((resolve) => app.once('exit', resolve))
 await rm(directory, { recursive: true, force: true })
}
