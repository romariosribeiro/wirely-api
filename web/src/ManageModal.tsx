import { useEffect, useRef, useState, type FormEvent } from 'react'
import { request, statusLabel, type Instance, type InstanceSettings, type ProxyConfig, type WebhookConfig } from './api'

const eventOptions = [
  { id: 'messages', title: 'Mensagens · todos os eventos', description: 'Recebidas, enviadas, edições, exclusões, reações e confirmações de entrega/leitura.' },
  { id: 'connection', title: 'Conexão da instância', description: 'Conectando, aguardando QR, conectada, desconectada e erros.' },
  { id: 'status', title: 'Status do WhatsApp', description: 'Publicações do Status recebidas ou enviadas pela conta.' },
  { id: 'presence', title: 'Presença', description: 'Online, offline, digitando e gravando, quando informados pelo WhatsApp.' },
  { id: 'groups', title: 'Grupos', description: 'Participantes, administradores, nome e descrição do grupo.' },
  { id: 'history', title: 'Sincronização do histórico', description: 'Progresso, tipo e quantidade de conversas recebidas na sincronização.' },
  { id: 'calls', title: 'Chamadas', description: 'Oferta, aceite, rejeição e encerramento de chamadas.' },
  { id: 'labels', title: 'Etiquetas', description: 'Edição de etiquetas e associações com conversas ou mensagens.' },
  { id: 'contacts', title: 'Contatos', description: 'Alterações sincronizadas na agenda do WhatsApp.' },
  { id: 'newsletters', title: 'Newsletters', description: 'Entrada, saída, silenciamento e atualizações ao vivo de canais.' },
]

type Props = {
  instance: Instance
  onClose: () => void
  onChanged: () => Promise<void>
  onDeleted: () => void
}

export function ManageModal({ instance, onClose, onChanged, onDeleted }: Props) {
  const dialog = useRef<HTMLDialogElement>(null)
  const tokenInput = useRef<HTMLInputElement>(null)
  const secretInput = useRef<HTMLInputElement>(null)
  const [token, setToken] = useState(instance.apiToken ?? '')
  const [tokenLoading, setTokenLoading] = useState(true)
  const [tokenError, setTokenError] = useState('')
  const [tokenLoadVersion, setTokenLoadVersion] = useState(0)
  const [config, setConfig] = useState<WebhookConfig | null>(null)
  const [url, setURL] = useState('')
  const [enabled, setEnabled] = useState(false)
  const [events, setEvents] = useState<string[]>([])
  const [secret, setSecret] = useState('')
  const [busy, setBusy] = useState('')
  const [error, setError] = useState('')
  const [notice, setNotice] = useState('')
  const [loadVersion, setLoadVersion] = useState(0)
  const [proxy, setProxy] = useState<ProxyConfig | null>(null)
  const [proxyURL, setProxyURL] = useState('')
  const [settings, setSettings] = useState<InstanceSettings | null>(null)
  const [settingsDraft, setSettingsDraft] = useState<InstanceSettings>({
    alwaysOnline: instance.alwaysOnline ?? false,
    rejectCall: instance.rejectCall ?? false,
    msgRejectCall: instance.msgRejectCall ?? '',
    readMessages: instance.readMessages ?? false,
    ignoreGroups: instance.ignoreGroups ?? false,
    ignoreStatus: instance.ignoreStatus ?? false,
  })
  const endpoint = `/api/instances/${instance.id}`
  const dirty = config !== null && (url !== config.url || enabled !== config.enabled ||
    JSON.stringify([...events].sort()) !== JSON.stringify([...config.events].sort()))
  const settingsDirty = settings !== null && JSON.stringify(settingsDraft) !== JSON.stringify(settings)
  const proxyDirty = proxyURL.trim() !== ''

  useEffect(() => {
    const element = dialog.current!
    const previousFocus = document.activeElement as HTMLElement | null
    element.showModal()
    return () => { element.close(); previousFocus?.focus() }
  }, [])

  useEffect(() => {
    let active = true
    request<WebhookConfig>(`${endpoint}/webhook`).then((value) => {
      if (!active) return
      setConfig(value)
      setURL(value.url)
      setEnabled(value.enabled)
      setEvents(value.events)
      setError('')
    }).catch((reason) => {
      if (active) setError(reason instanceof Error ? reason.message : 'Falha ao carregar webhook.')
    })
    return () => { active = false }
  }, [endpoint, loadVersion])

  useEffect(() => {
    const controller = new AbortController()
    request<{ token: string }>(`${endpoint}/token`, { signal: controller.signal }).then((value) => {
      if (!controller.signal.aborted) { setToken(value.token); setTokenError('') }
    }).catch(() => {
      if (!controller.signal.aborted) setTokenError('Não foi possível carregar o token. Tente novamente.')
    }).finally(() => {
      if (!controller.signal.aborted) setTokenLoading(false)
    })
    return () => controller.abort()
  }, [endpoint, tokenLoadVersion])

  useEffect(() => {
    let active = true
    request<InstanceSettings>(`${endpoint}/settings`).then((value) => {
      if (!active) return
      setSettings(value)
      setSettingsDraft(value)
    }).catch((reason) => {
      if (active) setError(reason instanceof Error ? reason.message : 'Falha ao carregar configurações.')
    })
    return () => { active = false }
  }, [endpoint])

  useEffect(() => {
    let active = true
    request<ProxyConfig>(`${endpoint}/proxy`).then((value) => {
      if (active) setProxy(value)
    }).catch((reason) => {
      if (active) setError(reason instanceof Error ? reason.message : 'Falha ao carregar proxy.')
    })
    return () => { active = false }
  }, [endpoint])

  function close() {
    if (busy) return
    if ((dirty || settingsDirty || proxyDirty) && !window.confirm('Descartar as alterações não salvas?')) return
    onClose()
  }

  async function copy(value: string, input: HTMLInputElement | null) {
    try {
      if (navigator.clipboard?.writeText) await navigator.clipboard.writeText(value)
      else {
        input?.focus()
        input?.select()
        if (!document.execCommand('copy')) throw new Error('copy failed')
      }
      setNotice('Copiado.')
    } catch {
      input?.focus()
      input?.select()
      setError('Selecione e copie o valor manualmente.')
    }
  }

  async function generateToken() {
    if (!window.confirm('Gerar um novo token? O anterior deixará de funcionar nas integrações.')) return
    setBusy('token'); setError(''); setNotice('')
    try {
      const result = await request<{ token: string }>(`${endpoint}/token`, { method: 'POST' })
      setToken(result.token)
      setTokenError('')
      setNotice('Token gerado. Ele continuará disponível neste painel.')
    } catch (reason) {
      setError(reason instanceof Error ? reason.message : 'Falha ao gerar token.')
    } finally { setBusy('') }
  }

  async function saveWebhook(event: FormEvent, rotateSecret = false) {
    event.preventDefault()
    if (config?.url && url !== config.url &&
        !window.confirm('Alterar a URL também gera um novo segredo de assinatura. Continuar?')) return
    if (rotateSecret && !window.confirm('Gerar um novo segredo de assinatura? Atualize-o no sistema receptor.')) return
    setBusy('webhook'); setError(''); setNotice('')
    try {
      const result = await request<WebhookConfig>(`${endpoint}/webhook`, {
        method: 'PUT', body: JSON.stringify({ url, enabled, events, rotateSecret }),
      })
      setConfig(result); setURL(result.url); setEnabled(result.enabled); setEvents(result.events)
      if (result.secret) setSecret(result.secret)
      else if (!result.hasSecret) setSecret('')
      setNotice(result.enabled ? 'Webhook salvo com os eventos selecionados.' : 'Webhook desativado. Configurações preservadas.')
    } catch (reason) {
      setError(reason instanceof Error ? reason.message : 'Falha ao salvar webhook.')
    } finally { setBusy('') }
  }

  async function saveSettings(event: FormEvent) {
    event.preventDefault()
    setBusy('settings'); setError(''); setNotice('')
    try {
      const result = await request<InstanceSettings>(`${endpoint}/settings`, {
        method: 'PUT', body: JSON.stringify(settingsDraft),
      })
      setSettings(result)
      setSettingsDraft(result)
      await onChanged()
      setNotice('Comportamento da instância atualizado.')
    } catch (reason) {
      setError(reason instanceof Error ? reason.message : 'Falha ao salvar configurações.')
    } finally { setBusy('') }
  }

  async function saveProxy(event: FormEvent) {
    event.preventDefault()
    if (proxy?.configured && !window.confirm('Trocar o proxy reconectará a instância. Continuar?')) return
    setBusy('proxy'); setError(''); setNotice('')
    try {
      const result = await request<ProxyConfig>(`${endpoint}/proxy`, {
        method: 'PUT', body: JSON.stringify({ url: proxyURL.trim() }),
      })
      setProxy(result)
      setProxyURL('')
      setNotice('Proxy salvo e aplicado à instância.')
    } catch (reason) {
      setError(reason instanceof Error ? reason.message : 'Falha ao salvar proxy.')
    } finally { setBusy('') }
  }

  async function removeProxy() {
    if (!window.confirm('Remover o proxy? A instância será reconectada diretamente.')) return
    setBusy('proxy'); setError(''); setNotice('')
    try {
      await request<void>(`${endpoint}/proxy`, { method: 'DELETE' })
      setProxy({ configured: false, hasPassword: false })
      setProxyURL('')
      setNotice('Proxy removido. A conexão direta foi restaurada.')
    } catch (reason) {
      setError(reason instanceof Error ? reason.message : 'Falha ao remover proxy.')
    } finally { setBusy('') }
  }

  function setSetting<K extends keyof InstanceSettings>(key: K, value: InstanceSettings[K]) {
    setSettingsDraft((current) => ({ ...current, [key]: value }))
  }

  async function instanceAction(action: 'disconnect' | 'delete') {
    const prompt = action === 'delete'
      ? `Excluir "${instance.name}" e sua sessão local? Esta ação não pode ser desfeita.`
      : `Desconectar "${instance.name}" do WhatsApp?`
    if (!window.confirm(prompt)) return
    setBusy(action); setError(''); setNotice('')
    try {
      await request(action === 'delete' ? endpoint : `${endpoint}/disconnect`, {
        method: action === 'delete' ? 'DELETE' : 'POST',
      })
      if (action === 'delete') onDeleted()
      else { await onChanged(); setNotice('Instância desconectada.') }
    } catch (reason) {
      setError(reason instanceof Error ? reason.message : 'Não foi possível concluir a ação.')
    } finally { setBusy('') }
  }

  function toggleEvent(id: string) {
    setEvents((current) => current.includes(id) ? current.filter((item) => item !== id) : [...current, id])
  }

  return (
    <dialog ref={dialog} className="manageModal" aria-labelledby="manage-title"
      onCancel={(event) => { event.preventDefault(); close() }}>
      <header className="manageHeader">
        <div>
          <p className="eyebrow">GERENCIAR INSTÂNCIA</p>
          <h2 id="manage-title">{instance.name}</h2>
          <span className={`badge badge-${instance.status}`}>{statusLabel(instance.status)}</span>
        </div>
        <button className="closeButton" type="button" aria-label="Fechar gerenciamento" disabled={!!busy} onClick={close}>×</button>
      </header>

      <div className="manageBody">
        {error && <p className="formError manageFeedback" role="alert">{error}</p>}
        {notice && <p className="manageSuccess manageFeedback" role="status">{notice}</p>}

        <section className="manageSection" aria-labelledby="token-section">
          <h3 id="token-section">Token da API</h3>
          <p>Use este token para identificar a instância nas suas integrações.</p>
          <div className="manageTokenRow">
            <input ref={tokenInput} aria-label="Token da API" value={token} readOnly
              placeholder={tokenLoading ? 'Carregando token…' : tokenError ? 'Token indisponível' : 'Token antigo: gere outro para exibir'} autoComplete="off" spellCheck={false} />
            <button className="secondaryButton" type="button" disabled={!token || !!busy}
              onClick={() => void copy(token, tokenInput.current)}>Copiar</button>
            <button className="primaryButton" type="button" disabled={!!busy || tokenLoading} aria-busy={busy === 'token'} onClick={() => void generateToken()}>
              {busy === 'token' ? 'Gerando…' : 'Gerar token'}
            </button>
          </div>
          {tokenError ? <p className="formError" role="alert">{tokenError} <button className="textButton" type="button"
            onClick={() => { setTokenLoading(true); setTokenLoadVersion((n) => n + 1) }}>Tentar novamente</button></p>
            : !token && !tokenLoading ? <small>Este token antigo foi salvo apenas como hash. Gere um novo uma vez para mantê-lo visível. O anterior será invalidado.</small>
            : <small>O token fica disponível ao reabrir este painel. Gerar outro invalida o anterior.</small>}
        </section>

        <form className="manageSection" onSubmit={(event) => void saveWebhook(event)}>
          <div className="manageSectionHeading">
            <div><h3>Webhook</h3><p>Escolha o que enviar para o seu sistema.</p></div>
            <label className="manageSwitch">
              <input type="checkbox" role="switch" checked={enabled} disabled={!config || !!busy}
                onChange={(event) => setEnabled(event.target.checked)} />
              <span>Webhook ativo</span>
            </label>
          </div>
          {!config ? (
            error ? <button type="button" className="secondaryButton" onClick={() => setLoadVersion((n) => n + 1)}>
              Tentar carregar novamente
            </button> : <div className="inlineLoading" role="status"><span className="spinner" aria-hidden="true" />Carregando configuração…</div>
          ) : (
            <>
              <label className="manageLabel" htmlFor="manage-webhook-url">URL do webhook</label>
              <input className="manageURL" id="manage-webhook-url" type="url" value={url}
                onChange={(event) => setURL(event.target.value)} placeholder="https://seu-sistema.com/webhook"
                required={enabled} maxLength={2048} disabled={!!busy} />
              <fieldset className="manageEvents" disabled={!!busy}>
                <legend>Eventos recebidos no webhook</legend>
                <div className="manageSelection">
                  <button type="button" className="manageSubtleButton" onClick={() => setEvents(eventOptions.map((item) => item.id))}>Selecionar todos</button>
                  <button type="button" className="manageSubtleButton" onClick={() => setEvents([])}>Limpar seleção</button>
                </div>
                {eventOptions.map((item) => (
                  <label className="manageEvent" key={item.id}>
                    <input type="checkbox" checked={events.includes(item.id)} onChange={() => toggleEvent(item.id)} />
                    <span><strong>{item.title}</strong><small>{item.description}</small></span>
                  </label>
                ))}
              </fieldset>
              {enabled && events.length === 0 && <p className="manageHint">Nenhum evento será enviado enquanto a seleção estiver vazia.</p>}
              {!enabled && <p className="manageHint">Ao salvar desativado, os envios param e sua configuração fica guardada.</p>}
              {secret && (
                <div className="manageSecret">
                  <label htmlFor="manage-webhook-secret">Novo segredo de assinatura — copie antes de fechar</label>
                  <div className="tokenField">
                    <input id="manage-webhook-secret" ref={secretInput} value={secret} readOnly autoComplete="off" />
                    <button className="secondaryButton" type="button" onClick={() => void copy(secret, secretInput.current)}>Copiar segredo</button>
                  </div>
                </div>
              )}
              <div className="manageSave">
                <span>{dirty ? 'Alterações não salvas' : 'Configuração salva'}</span>
                {config.hasSecret && <button type="button" className="manageSubtleButton" disabled={!!busy}
                  onClick={(event) => void saveWebhook(event, true)}>Renovar segredo</button>}
                <button className="primaryButton" type="submit" disabled={!!busy} aria-busy={busy === 'webhook'}>
                  {busy === 'webhook' ? 'Salvando…' : 'Salvar webhook'}
                </button>
              </div>
            </>
          )}
        </form>

        <form className="manageSection" onSubmit={(event) => void saveProxy(event)}>
          <div className="manageSectionHeading">
            <div><h3>Proxy da conexão</h3><p>Defina um proxy exclusivo para esta instância.</p></div>
            {proxy?.configured && <span className="manageProxyBadge">ATIVO</span>}
          </div>
          {!proxy ? <div className="inlineLoading" role="status"><span className="spinner" aria-hidden="true" />Carregando proxy…</div> : <>
            {proxy.configured && <div className="manageProxyCurrent">
              <span>Proxy atual</span>
              <code>{proxy.url}</code>
              <small>A senha é protegida e aparece mascarada.</small>
            </div>}
            <label className="manageLabel" htmlFor="manage-proxy-url">{proxy.configured ? 'Novo endereço do proxy' : 'Endereço do proxy'}</label>
            <input className="manageURL" id="manage-proxy-url" type="text" value={proxyURL}
              onChange={(event) => setProxyURL(event.target.value)}
              placeholder="socks5://usuario:senha@proxy.exemplo.com:1080"
              maxLength={2048} disabled={!!busy} autoComplete="off" spellCheck={false} />
            <p className="manageHint">Compatível com HTTP, HTTPS e SOCKS5. Usuário e senha são opcionais. Alterar ou remover reconecta a instância.</p>
            <div className="manageSave">
              <span>{proxy.configured ? `${proxy.scheme?.toUpperCase()} · ${proxy.host}:${proxy.port}` : 'Conexão direta, sem proxy'}</span>
              {proxy.configured && <button type="button" className="manageSubtleButton manageProxyRemove" disabled={!!busy}
                onClick={() => void removeProxy()}>Remover proxy</button>}
              <button className="primaryButton" type="submit" disabled={!!busy || !proxyDirty} aria-busy={busy === 'proxy'}>
                {busy === 'proxy' ? 'Aplicando…' : proxy.configured ? 'Trocar proxy' : 'Salvar proxy'}
              </button>
            </div>
          </>}
        </form>

        <form className="manageSection" onSubmit={(event) => void saveSettings(event)}>
          <div className="manageSectionHeading">
            <div><h3>Comportamento</h3><p>Automatize a presença, chamadas e o tratamento das mensagens recebidas.</p></div>
          </div>
          {!settings ? <div className="inlineLoading" role="status"><span className="spinner" aria-hidden="true" />Carregando configuração…</div> : <>
            <div className="manageBehaviorGrid">
              <label className="manageBehavior">
                <span><strong>Sempre online</strong><small>Mantém a presença da conta como disponível enquanto estiver conectada.</small></span>
                <span className="manageSwitch"><input type="checkbox" role="switch" checked={settingsDraft.alwaysOnline} disabled={!!busy}
                  onChange={(event) => setSetting('alwaysOnline', event.target.checked)} /><span>{settingsDraft.alwaysOnline ? 'Ligado' : 'Desligado'}</span></span>
              </label>
              <label className="manageBehavior">
                <span><strong>Rejeitar chamadas</strong><small>Recusa automaticamente chamadas recebidas nesta instância.</small></span>
                <span className="manageSwitch"><input type="checkbox" role="switch" checked={settingsDraft.rejectCall} disabled={!!busy}
                  onChange={(event) => setSetting('rejectCall', event.target.checked)} /><span>{settingsDraft.rejectCall ? 'Ligado' : 'Desligado'}</span></span>
              </label>
              <label className="manageBehavior">
                <span><strong>Ler mensagens</strong><small>Envia confirmação de leitura automaticamente ao receber mensagens.</small></span>
                <span className="manageSwitch"><input type="checkbox" role="switch" checked={settingsDraft.readMessages} disabled={!!busy}
                  onChange={(event) => setSetting('readMessages', event.target.checked)} /><span>{settingsDraft.readMessages ? 'Ligado' : 'Desligado'}</span></span>
              </label>
              <label className="manageBehavior">
                <span><strong>Ignorar grupos</strong><small>Não processa nem encaminha eventos de mensagens de grupos.</small></span>
                <span className="manageSwitch"><input type="checkbox" role="switch" checked={settingsDraft.ignoreGroups} disabled={!!busy}
                  onChange={(event) => setSetting('ignoreGroups', event.target.checked)} /><span>{settingsDraft.ignoreGroups ? 'Ligado' : 'Desligado'}</span></span>
              </label>
              <label className="manageBehavior">
                <span><strong>Ignorar Status</strong><small>Não processa nem encaminha publicações do Status do WhatsApp.</small></span>
                <span className="manageSwitch"><input type="checkbox" role="switch" checked={settingsDraft.ignoreStatus} disabled={!!busy}
                  onChange={(event) => setSetting('ignoreStatus', event.target.checked)} /><span>{settingsDraft.ignoreStatus ? 'Ligado' : 'Desligado'}</span></span>
              </label>
            </div>
            <label className="manageLabel" htmlFor="manage-reject-message">Mensagem ao rejeitar chamada</label>
            <textarea id="manage-reject-message" className="manageRejectMessage" rows={3} maxLength={1000}
              value={settingsDraft.msgRejectCall} disabled={!!busy || !settingsDraft.rejectCall}
              onChange={(event) => setSetting('msgRejectCall', event.target.value)}
              placeholder="Ex.: Não podemos atender chamadas. Envie uma mensagem por aqui." />
            <div className="manageSave">
              <span>{settingsDirty ? 'Alterações não salvas' : 'Configuração salva'}</span>
              <button className="primaryButton" type="submit" disabled={!!busy || !settingsDirty} aria-busy={busy === 'settings'}>
                {busy === 'settings' ? 'Salvando…' : 'Salvar comportamento'}
              </button>
            </div>
          </>}
        </form>

        <section className="manageSection manageDanger" aria-labelledby="instance-actions">
          <h3 id="instance-actions">Ações da instância</h3>
          <div className="manageActionRow">
            <div><strong>Desconectar</strong><p>Interrompe a conexão atual com o WhatsApp.</p></div>
            <button className="secondaryButton" type="button"
              disabled={!!busy || ['disconnected', 'logged_out', 'error'].includes(instance.status)}
              aria-busy={busy === 'disconnect'} onClick={() => void instanceAction('disconnect')}>
              {busy === 'disconnect' ? 'Desconectando…' : 'Desconectar'}
            </button>
          </div>
          <div className="manageActionRow">
            <div><strong>Excluir instância</strong><p>Remove a instância, o token e a sessão local.</p></div>
            <button className="manageDelete" type="button" disabled={!!busy} aria-busy={busy === 'delete'} onClick={() => void instanceAction('delete')}>
              {busy === 'delete' ? 'Excluindo…' : 'Excluir'}
            </button>
          </div>
        </section>
      </div>
      <footer className="manageFooter">
        <button className="secondaryButton" type="button" disabled={!!busy} onClick={close}>Fechar</button>
      </footer>
    </dialog>
  )
}
