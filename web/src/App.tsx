import { FormEvent, useCallback, useEffect, useState } from 'react'

import { can, request, statusLabel, type Instance, type ConnectionState, type MetricsSnapshot, type UpdateStatus, type User, type UserRole } from './api'
import { ManageModal } from './ManageModal'
import { PairingModal } from './PairingModal'
import { LoadingState } from './LoadingState'
import { TesterModal } from './TesterModal'
import { DocsModal } from './DocsModal'
import { ActivityModal } from './ActivityModal'
import { InboxModal } from './InboxModal'
import { UsersModal } from './UsersModal'
import { MetricsModal } from './MetricsModal'
import { OperationsModal } from './OperationsModal'
import { UpdateModal } from './UpdateModal'

const roleLabels: Record<UserRole, string> = { owner: 'Proprietário', admin: 'Administrador', operator: 'Operador', viewer: 'Visualizador' }

function Login({ onAuthenticated }: { onAuthenticated: () => void }) {
  const [username, setUsername] = useState('admin')
  const [password, setPassword] = useState('')
  const [error, setError] = useState('')
  const [busy, setBusy] = useState(false)

  async function submit(event: FormEvent) {
    event.preventDefault()
    setBusy(true)
    setError('')
    try {
      await request('/api/auth/login', {
        method: 'POST',
        body: JSON.stringify({ username, password }),
      })
      onAuthenticated()
    } catch (reason) {
      setError(reason instanceof Error ? reason.message : 'Falha ao entrar')
    } finally {
      setBusy(false)
    }
  }

  return (
    <main className="loginPage">
      <section className="loginCard">
        <a className="brand" href="/" aria-label="Wirely API">
          <span className="brandMark">W</span>
          <span>Wirely <span className="brandSuffix">API</span></span>
        </a>
        <div className="loginCopy">
          <p className="eyebrow">PAINEL DE GERENCIAMENTO</p>
          <h1>Bem-vindo.</h1>
          <p>Entre com sua credencial individual do Wirely.</p>
        </div>
        <form onSubmit={submit}>
          <label htmlFor="username">Usuário</label>
          <input id="username" type="text" autoComplete="username" value={username}
            onChange={(event) => setUsername(event.target.value)} minLength={3} maxLength={40} required />
          <label htmlFor="password">Senha</label>
          <input
            id="password"
            type="password"
            autoComplete="current-password"
            value={password}
            onChange={(event) => setPassword(event.target.value)}
            required
            autoFocus
          />
          {error && <p className="formError">{error}</p>}
          <button type="submit" disabled={busy} aria-busy={busy}>
            {busy ? 'Entrando…' : 'Entrar'}
          </button>
        </form>
      </section>
    </main>
  )
}

function PasswordModal({ onClose, onChanged }: { onClose: () => void; onChanged: () => void }) {
  const [currentPassword, setCurrentPassword] = useState('')
  const [newPassword, setNewPassword] = useState('')
  const [confirmation, setConfirmation] = useState('')
  const [error, setError] = useState('')
  const [busy, setBusy] = useState(false)

  async function submit(event: FormEvent) {
    event.preventDefault()
    if (newPassword !== confirmation) {
      setError('A confirmação não corresponde à nova senha.')
      return
    }
    setBusy(true)
    setError('')
    try {
      await request('/api/auth/password', {
        method: 'PUT',
        body: JSON.stringify({ currentPassword, newPassword }),
      })
      onChanged()
    } catch (reason) {
      setError(reason instanceof Error ? reason.message : 'Falha ao alterar a senha')
    } finally {
      setBusy(false)
    }
  }

  return (
    <div className="modalBackdrop" role="presentation">
      <section className="pairingModal passwordModal" role="dialog" aria-modal="true" aria-labelledby="password-title">
        <div className="modalHeader">
          <div>
            <p className="eyebrow">SEGURANÇA</p>
            <h2 id="password-title">Alterar senha</h2>
          </div>
          <button className="closeButton" type="button" aria-label="Fechar" onClick={onClose}>×</button>
        </div>
        <form className="passwordForm" onSubmit={submit}>
          <label htmlFor="current-password">Senha atual</label>
          <input
            id="current-password"
            type="password"
            autoComplete="current-password"
            value={currentPassword}
            onChange={(event) => setCurrentPassword(event.target.value)}
            required
            autoFocus
          />
          <label htmlFor="new-password">Nova senha</label>
          <input
            id="new-password"
            type="password"
            autoComplete="new-password"
            value={newPassword}
            onChange={(event) => setNewPassword(event.target.value)}
            minLength={12}
            maxLength={128}
            required
          />
          <label htmlFor="confirm-password">Confirmar nova senha</label>
          <input
            id="confirm-password"
            type="password"
            autoComplete="new-password"
            value={confirmation}
            onChange={(event) => setConfirmation(event.target.value)}
            minLength={12}
            maxLength={128}
            required
          />
          <small>Use entre 12 e 128 caracteres. Todas as sessões abertas serão encerradas.</small>
          {error && <p className="formError">{error}</p>}
          <div className="passwordActions">
            <button className="secondaryButton" type="button" onClick={onClose}>Cancelar</button>
            <button className="primaryButton" type="submit" disabled={busy} aria-busy={busy}>
              {busy ? 'Alterando…' : 'Alterar senha'}
            </button>
          </div>
        </form>
      </section>
    </div>
  )
}


function Dashboard({
  user,
  onLogout,
  onPasswordChanged,
}: { user: User; onLogout: () => void; onPasswordChanged: () => void }) {
  const [instances, setInstances] = useState<Instance[]>([])
  const [name, setName] = useState('')
  const [error, setError] = useState('')
  const [busy, setBusy] = useState(false)
  const [selected, setSelected] = useState<Instance | null>(null)
  const [connection, setConnection] = useState<ConnectionState | null>(null)
  const [hasShownQR, setHasShownQR] = useState(false)
  const [connectPending, setConnectPending] = useState(false)
  const [connectReady, setConnectReady] = useState(false)
  const [managed, setManaged] = useState<Instance | null>(null)
  const [tested, setTested] = useState<Instance | null>(null)
  const [activity, setActivity] = useState<Instance | null>(null)
  const [inbox, setInbox] = useState<Instance | null>(null)
  const [passwordModalOpen, setPasswordModalOpen] = useState(false)
  const [docsOpen, setDocsOpen] = useState(false)
  const [usersOpen, setUsersOpen] = useState(false)
  const [metricsOpen, setMetricsOpen] = useState(false)
  const [operationsOpen, setOperationsOpen] = useState(false)
  const [updateOpen, setUpdateOpen] = useState(false)
  const [metrics, setMetrics] = useState<MetricsSnapshot | null>(null)
  const [updateStatus, setUpdateStatus] = useState<UpdateStatus | null>(null)
  const canAdmin = can(user, 'admin')
  const canOperate = can(user, 'operator')
  const isOwner = user.role === 'owner'

  const loadInstances = useCallback(async () => {
    try {
      const response = await request<{ data: Instance[] }>('/api/instances')
      setInstances(response.data)
    } catch (reason) {
      setError(reason instanceof Error ? reason.message : 'Falha ao carregar instâncias')
    }
  }, [])

  useEffect(() => {
    let active = true
    request<{ data: Instance[] }>('/api/instances')
      .then((response) => {
        if (active) setInstances(response.data)
      })
      .catch((reason) => {
        if (active) setError(reason instanceof Error ? reason.message : 'Falha ao carregar instâncias')
      })
    const timer = window.setInterval(() => {
      void loadInstances()
    }, 5000)
    return () => {
      active = false
      window.clearInterval(timer)
    }
  }, [loadInstances])

  useEffect(() => {
    let active = true
    const loadMetrics = async () => {
      try {
        const value = await request<MetricsSnapshot>('/api/metrics?range=24h')
        if (active) setMetrics(value)
      } catch {
        // Instance controls remain usable if operational metrics are temporarily unavailable.
      }
    }
    void loadMetrics()
    const timer = window.setInterval(() => void loadMetrics(), 15000)
    return () => {
      active = false
      window.clearInterval(timer)
    }
  }, [])

  useEffect(() => {
    if (!isOwner) return
    let active = true
    const loadUpdate = async () => {
      try {
        const value = await request<UpdateStatus>('/api/system/update')
        if (active) setUpdateStatus(value)
      } catch {
        // The dashboard remains available when GitHub cannot be reached.
      }
    }
    void loadUpdate()
    const timer = window.setInterval(() => void loadUpdate(), 15 * 60 * 1000)
    return () => { active = false; window.clearInterval(timer) }
  }, [isOwner])

  useEffect(() => {
    if (!selected || !connectReady) return
    let active = true
    let timer: number | undefined
    const controller = new AbortController()
    const poll = async () => {
      try {
        const state = await request<ConnectionState>(`/api/instances/${selected.id}/state`, { signal: controller.signal })
        if (!active) return
        setConnection(state)
        if (state.qrAvailable) setHasShownQR(true)
        void loadInstances()
        if (['connected', 'error', 'disconnected', 'logged_out'].includes(state.status)) return
      } catch (reason) {
        if (!active) return
        setConnection({ status: 'error', qrAvailable: false, lastError: reason instanceof Error ? reason.message : 'Falha ao consultar conexão' })
        return
      }
      if (active) timer = window.setTimeout(() => void poll(), 2000)
    }
    void poll()
    return () => {
      active = false
      controller.abort()
      window.clearTimeout(timer)
    }
  }, [selected, connectReady, loadInstances])

  async function createInstance(event: FormEvent) {
    event.preventDefault()
    setBusy(true)
    setError('')
    try {
      const instance = await request<Instance>('/api/instances', {
        method: 'POST',
        body: JSON.stringify({ name }),
      })
      setManaged(instance)
      setName('')
      await loadInstances()
    } catch (reason) {
      setError(reason instanceof Error ? reason.message : 'Falha ao criar instância')
    } finally {
      setBusy(false)
    }
  }

  async function connectInstance(instance: Instance) {
    setError('')
    setHasShownQR(false)
    setConnectPending(true)
    setConnectReady(false)
    setSelected(instance)
    setConnection({ status: 'connecting', qrAvailable: false })
    try {
      await request(`/api/instances/${instance.id}/connect`, { method: 'POST' })
      setConnectReady(true)
    } catch (reason) {
      setError(reason instanceof Error ? reason.message : 'Falha ao iniciar conexão')
      setConnection({ status: 'error', qrAvailable: false, lastError: reason instanceof Error ? reason.message : 'Falha ao iniciar conexão' })
    } finally {
      setConnectPending(false)
    }
  }

  async function closePairing() {
    if (selected && connection?.status !== 'connected') {
      await request(`/api/instances/${selected.id}/disconnect`, { method: 'POST' })
    }
    setSelected(null)
    setConnection(null)
    await loadInstances()
  }

  async function disconnectInstance(instance: Instance) {
    if (!window.confirm(`Desconectar ${instance.name} do WhatsApp?`)) return
    setError('')
    try {
      await request(`/api/instances/${instance.id}/disconnect`, { method: 'POST' })
      await loadInstances()
    } catch (reason) {
      setError(reason instanceof Error ? reason.message : 'Falha ao desconectar a instância')
    }
  }

  return (
    <main className="shell">
      <header className="topbar">
        <a className="brand" href="/" aria-label="Wirely API">
          <span className="brandMark">W</span>
          <span>Wirely <span className="brandSuffix">API</span></span>
        </a>
        <div className="topActions">
          <span className="status online">API online</span>
          <button className="textButton" type="button" onClick={() => setMetricsOpen(true)}>Métricas</button>
          <button className="textButton" type="button" onClick={() => setOperationsOpen(true)}>Operações</button>
          <button className="textButton docsNavButton" type="button" onClick={() => setDocsOpen(true)}>
            <span>Documentação</span><span>Docs</span>
          </button>
          {isOwner && <button className="textButton" type="button" onClick={() => setUsersOpen(true)}>Equipe</button>}
          <button className="textButton" type="button" onClick={() => setPasswordModalOpen(true)}>Segurança</button>
          <span className="currentUser" title={roleLabels[user.role]}><b>{user.username}</b><small>{roleLabels[user.role]}</small></span>
          <button className="textButton" type="button" onClick={onLogout}>Sair</button>
        </div>
      </header>

      <section className="pageHeading">
        <div>
          <p className="eyebrow">VISÃO GERAL</p>
          <h1>Instâncias</h1>
          <p>Suas conexões com o WhatsApp, em um só lugar.</p>
        </div>
{canAdmin && (        <form className="createForm" onSubmit={createInstance}>
          <input
            aria-label="Nome da instância"
            placeholder="Ex.: Atendimento"
            value={name}
            onChange={(event) => setName(event.target.value)}
            minLength={2}
            maxLength={60}
            required
          />
          <button type="submit" disabled={busy} aria-busy={busy}>{busy ? 'Criando…' : 'Nova instância'}</button>
        </form>)}
      </section>

      {error && <p className="pageError">{error}</p>}

      <section className="summary" aria-label="Resumo">
        <article>
          <span>Instâncias</span>
          <strong>{instances.length}</strong>
          <small>{instances.length === 1 ? 'conexão cadastrada' : 'conexões cadastradas'}</small>
        </article>
        <article>
          <span>Conectadas</span>
          <strong>{instances.filter((item) => item.status === 'connected').length}</strong>
          <small>via WhatsApp Web</small>
        </article>
        <article>
          <span>Mensagens · 24h</span>
          <strong>{metrics ? metrics.messages.total : '—'}</strong>
          <small>{metrics ? `${metrics.messages.received} recebidas · ${metrics.messages.sent} enviadas` : 'calculando atividade'}</small>
        </article>
        <article>
          <span>Fila pendente</span>
          <strong className={metrics && metrics.queue.pending > 0 ? 'metricWarn' : ''}>{metrics ? metrics.queue.pending : '—'}</strong>
          <small>{metrics ? `${metrics.queue.failed} falhas nas últimas 24h` : 'calculando envios'}</small>
        </article>
      </section>

      {isOwner && updateStatus && <button type="button"
        className={`dashboardUpdate ${updateStatus.updateAvailable ? 'available' : ''}`}
        onClick={() => setUpdateOpen(true)}>
        <span className="dashboardUpdateIcon" aria-hidden="true">{updateStatus.updateAvailable ? '↓' : '✓'}</span>
        <span><small>{updateStatus.updateAvailable ? 'ATUALIZAÇÃO DISPONÍVEL' : 'SISTEMA ATUALIZADO'}</small>
          <strong>{updateStatus.updateAvailable
            ? `Wirely v${updateStatus.latestVersion}`
            : `Wirely v${updateStatus.currentVersion}`}</strong>
          <em>{updateStatus.updateAvailable ? 'Veja as mudanças e atualize com backup automático.' : 'Clique para verificar versões e segurança da instalação.'}</em></span>
        <b>{updateStatus.updateAvailable ? 'Ver atualização' : 'Verificar'} <span aria-hidden="true">→</span></b>
      </button>}

      <section className="instanceSection">
        <div className="sectionTitle">
          <h2>Suas instâncias</h2>
          <span>{instances.length}</span>
        </div>
        {instances.length === 0 ? (
          <div className="emptyState">
            <span className="emptyIcon">+</span>
            <h3>Nenhuma instância criada</h3>
            <p>Crie a primeira instância para configurar uma conexão.</p>
          </div>
        ) : (
          <div className="instanceList">
            {instances.map((instance) => (
              <article className="instanceRow" key={instance.id}>
                <div className="instanceIdentity">
                  <span className="instanceAvatar">{instance.name.slice(0, 1).toUpperCase()}</span>
                  <div>
                    <h3>{instance.name}</h3>
                    <small>Criada em {new Date(instance.createdAt).toLocaleDateString('pt-BR')}</small>
                  </div>
                </div>
                <div className="instanceActions">
                  <span className={`badge badge-${instance.status}`}>{statusLabel(instance.status)}</span>
                  {canOperate && instance.status !== 'connected' && (
                    <button className="secondaryButton" type="button" onClick={() => void connectInstance(instance)}>Conectar</button>
                  )}
                  {canOperate && instance.status === 'connected' && <button className="secondaryButton" type="button" onClick={() => void disconnectInstance(instance)}>Desconectar</button>}
                  {canAdmin && <button className="secondaryButton" type="button" disabled={instance.status !== 'connected'}
                    title={instance.status !== 'connected' ? 'Conecte a instância para testar envios' : undefined}
                    onClick={() => setTested(instance)}>Testar API</button>}
                  <button className="secondaryButton inboxButton" type="button" onClick={() => setInbox(instance)}>Conversas</button>
                  <button className="secondaryButton activityButton" type="button" onClick={() => setActivity(instance)}>Atividade</button>
                  {canAdmin && <button className="primaryButton" type="button" onClick={() => setManaged(instance)}>Gerenciar</button>}
                </div>
              </article>
            ))}
          </div>
        )}
      </section>


      {metricsOpen && <MetricsModal onClose={() => setMetricsOpen(false)} />}
      {operationsOpen && <OperationsModal user={user} onClose={() => setOperationsOpen(false)} />}
      {updateOpen && updateStatus && <UpdateModal initial={updateStatus} onClose={() => setUpdateOpen(false)} onChanged={setUpdateStatus} />}
      {docsOpen && <DocsModal instances={instances} canManage={canAdmin} onClose={() => setDocsOpen(false)}
        onManage={(instance) => { setDocsOpen(false); setManaged(instance) }} />}
      {usersOpen && <UsersModal onClose={() => setUsersOpen(false)} />}
      {passwordModalOpen && (
        <PasswordModal
          onClose={() => setPasswordModalOpen(false)}
          onChanged={onPasswordChanged}
        />
      )}
      {inbox && <InboxModal key={inbox.id} canSend={canOperate} instance={{ ...inbox, status: instances.find((item) => item.id === inbox.id)?.status ?? inbox.status }} onClose={() => setInbox(null)} />}
      {activity && <ActivityModal key={activity.id} instance={activity} canOperate={canOperate} onClose={() => setActivity(null)} />}
      {tested && <TesterModal key={tested.id} instance={tested} onClose={() => setTested(null)}
        onManage={() => { setTested(null); setManaged(tested) }} />}
      {managed && (
        <ManageModal
          key={managed.id}
          instance={{ ...managed, status: instances.find((item) => item.id === managed.id)?.status ?? managed.status }}
          onClose={() => setManaged(null)}
          onChanged={loadInstances}
          onDeleted={() => { setManaged(null); void loadInstances() }}
        />
      )}

      {selected && <PairingModal key={selected.id} instance={selected} connection={connection}
        hasShownQR={hasShownQR} starting={connectPending} onClose={closePairing} />}

    </main>
  )
}

export default function App() {
  const [state, setState] = useState<'checking' | 'guest' | 'authenticated'>('checking')
  const [user, setUser] = useState<User | null>(null)

  const authenticate = useCallback(async () => {
    try {
      const current = await request<User>('/api/auth/me')
      setUser(current); setState('authenticated')
    } catch {
      setUser(null); setState('guest')
    }
  }, [])

  useEffect(() => {
    const timer = window.setTimeout(() => void authenticate(), 0)
    return () => window.clearTimeout(timer)
  }, [authenticate])
  useEffect(() => {
    const revoked = () => { setUser(null); setState('guest') }
    window.addEventListener('wirely:unauthorized', revoked)
    return () => window.removeEventListener('wirely:unauthorized', revoked)
  }, [])

  async function logout() {
    await request('/api/auth/logout', { method: 'POST' }).catch(() => undefined)
    setUser(null); setState('guest')
  }

  if (state === 'checking') return <div className="loading"><LoadingState title="Carregando Wirely API" description="Preparando seu painel de gerenciamento." /></div>
  if (state === 'guest') return <Login onAuthenticated={() => void authenticate()} />
  if (!user) return null
  return <Dashboard user={user} onLogout={() => void logout()} onPasswordChanged={() => { setUser(null); setState('guest') }} />
}
