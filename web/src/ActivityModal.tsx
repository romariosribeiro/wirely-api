import { useCallback, useEffect, useRef, useState } from 'react'
import { request, type ActivityEvent, type Instance, type MessageJob, type Page, type WebhookDelivery } from './api'

type View = 'events' | 'webhooks' | 'queue'

const eventLabels: Record<string, string> = {
  'message.received': 'Mensagem recebida', 'message.sent': 'Mensagem enviada',
  'message.updated': 'Mensagem editada', 'message.deleted': 'Mensagem apagada',
  'message.reaction': 'Reação', 'message.receipt': 'Confirmação de leitura',
  'instance.status': 'Conexão alterada', 'presence.updated': 'Presença atualizada',
  'group.updated': 'Grupo atualizado', 'status.received': 'Status recebido', 'status.sent': 'Status enviado',
}

const jobStatusLabels: Record<MessageJob['status'], string> = {
  queued: 'Agendada', processing: 'Enviando', retrying: 'Nova tentativa',
  sent: 'Enviada', failed: 'Falhou', canceled: 'Cancelada',
}

const jobKindLabels: Record<MessageJob['kind'], string> = {
  text: 'Texto', image: 'Imagem', video: 'Vídeo', audio: 'Áudio', document: 'Documento', sticker: 'Figurinha',
}

function dateTime(value: string) {
  return new Intl.DateTimeFormat('pt-BR', { dateStyle: 'short', timeStyle: 'medium' }).format(new Date(value))
}

function value(data: Record<string, unknown>, key: string) {
  const current = data[key]
  return typeof current === 'string' ? current : ''
}

function eventDescription(item: ActivityEvent) {
  const text = value(item.data, 'text')
  if (text) return text
  const chat = value(item.data, 'chat') || value(item.data, 'from')
  if (chat) return chat
  return value(item.data, 'status') || 'Evento processado pela instância.'
}

function jobDescription(item: MessageJob) {
  return item.payload.message || item.payload.caption || item.payload.fileName || jobKindLabels[item.kind]
}

function fileSize(value?: number) {
  if (!value) return ''
  return value >= 1024 * 1024 ? `${(value / 1024 / 1024).toFixed(1)} MB` : `${Math.ceil(value / 1024)} KB`
}

export function ActivityModal({ instance, canOperate, onClose }: { instance: Instance; canOperate: boolean; onClose: () => void }) {
  const dialog = useRef<HTMLDialogElement>(null)
  const [view, setView] = useState<View>('events')
  const [filter, setFilter] = useState('all')
  const [page, setPage] = useState(1)
  const [events, setEvents] = useState<Page<ActivityEvent> | null>(null)
  const [deliveries, setDeliveries] = useState<Page<WebhookDelivery> | null>(null)
  const [jobs, setJobs] = useState<Page<MessageJob> | null>(null)
  const [loading, setLoading] = useState(true)
  const [refreshing, setRefreshing] = useState(false)
  const [retrying, setRetrying] = useState<number | null>(null)
  const [busyJob, setBusyJob] = useState<string | null>(null)
  const [error, setError] = useState('')
  const [notice, setNotice] = useState('')

  useEffect(() => {
    const element = dialog.current!
    const previousFocus = document.activeElement as HTMLElement | null
    element.showModal()
    return () => { element.close(); previousFocus?.focus() }
  }, [])

  const load = useCallback(async (quiet = false, signal?: AbortSignal) => {
    if (quiet) setRefreshing(true); else setLoading(true)
    setError('')
    const query = new URLSearchParams({ page: String(page), pageSize: '20' })
    try {
      if (view === 'events') {
        query.set('category', filter)
        setEvents(await request<Page<ActivityEvent>>(`/api/v1/instances/${instance.id}/events?${query}`, { signal }))
      } else if (view === 'webhooks') {
        query.set('status', filter)
        setDeliveries(await request<Page<WebhookDelivery>>(`/api/v1/instances/${instance.id}/webhook-deliveries?${query}`, { signal }))
      } else {
        query.set('status', filter)
        setJobs(await request<Page<MessageJob>>(`/api/v1/instances/${instance.id}/queue?${query}`, { signal }))
      }
    } catch (reason) {
      if (!signal?.aborted) setError(reason instanceof Error ? reason.message : 'Falha ao carregar o histórico.')
    } finally {
      if (!signal?.aborted) { setLoading(false); setRefreshing(false) }
    }
  }, [filter, instance.id, page, view])

  useEffect(() => {
    const controller = new AbortController()
    const initial = window.setTimeout(() => void load(false, controller.signal), 0)
    const timer = window.setInterval(() => void load(true, controller.signal), 8000)
    return () => { controller.abort(); window.clearTimeout(initial); window.clearInterval(timer) }
  }, [load])

  function changeView(next: View) {
    setView(next); setFilter('all'); setPage(1); setNotice('')
  }

  function changeFilter(next: string) {
    setFilter(next); setPage(1); setNotice('')
  }

  async function retryWebhook(item: WebhookDelivery) {
    if (!window.confirm('Reenviar este evento para o webhook configurado atualmente?')) return
    setRetrying(item.id); setError(''); setNotice('')
    try {
      await request(`/api/v1/instances/${instance.id}/webhook-deliveries/${item.id}/retry`, { method: 'POST' })
      setNotice('Webhook entregue com sucesso. A nova tentativa foi adicionada ao histórico.')
      await load(true)
    } catch (reason) {
      setError(reason instanceof Error ? reason.message : 'Não foi possível reenviar o webhook.')
      await load(true)
    } finally { setRetrying(null) }
  }

  async function mutateJob(item: MessageJob, action: 'cancel' | 'retry') {
    const question = action === 'cancel' ? 'Cancelar este envio agendado?' : 'Colocar este envio novamente na fila?'
    if (!window.confirm(question)) return
    setBusyJob(item.id); setError(''); setNotice('')
    try {
      await request(`/api/v1/instances/${instance.id}/queue/${item.id}${action === 'retry' ? '/retry' : ''}`, {
        method: action === 'retry' ? 'POST' : 'DELETE',
      })
      setNotice(action === 'retry' ? 'Envio recolocado na fila.' : 'Envio cancelado com segurança.')
      await load(true)
    } catch (reason) {
      setError(reason instanceof Error ? reason.message : 'Não foi possível atualizar o envio.')
      await load(true)
    } finally { setBusyJob(null) }
  }

  const result = view === 'events' ? events : view === 'webhooks' ? deliveries : jobs
  const items = result?.data ?? []
  const busy = retrying !== null || busyJob !== null

  return (
    <dialog ref={dialog} className="activityModal" aria-labelledby="activity-title"
      onCancel={(event) => { event.preventDefault(); if (!busy) onClose() }}>
      <header className="manageHeader activityHeader">
        <div><p className="eyebrow">OBSERVABILIDADE</p><h2 id="activity-title">Atividade · {instance.name}</h2>
          <span className="activityLive"><i /> atualização automática</span></div>
        <button className="closeButton" type="button" aria-label="Fechar histórico" disabled={busy} onClick={onClose}>×</button>
      </header>

      <div className="activityToolbar">
        <div className="activityTabs" role="tablist" aria-label="Tipo de histórico">
          <button type="button" role="tab" aria-selected={view === 'events'} onClick={() => changeView('events')}>Eventos</button>
          <button type="button" role="tab" aria-selected={view === 'webhooks'} onClick={() => changeView('webhooks')}>Webhooks</button>
          <button type="button" role="tab" aria-selected={view === 'queue'} onClick={() => changeView('queue')}>Fila</button>
        </div>
        <label><span className="visuallyHidden">Filtrar histórico</span>
          <select value={filter} onChange={(event) => changeFilter(event.target.value)}>
            <option value="all">Todos</option>
            {view === 'events' ? <>
              <option value="messages">Mensagens</option><option value="connection">Conexão</option>
              <option value="status">Status</option><option value="presence">Presença</option><option value="groups">Grupos</option>
            </> : view === 'webhooks' ? <><option value="failed">Com falha</option><option value="delivered">Entregues</option></> : <>
              <option value="queued">Agendadas</option><option value="processing">Enviando</option><option value="retrying">Tentando novamente</option>
              <option value="sent">Enviadas</option><option value="failed">Com falha</option><option value="canceled">Canceladas</option>
            </>}
          </select>
        </label>
        <button className="activityRefresh" type="button" disabled={refreshing || loading} onClick={() => void load(true)} aria-label="Atualizar histórico">
          <span className={refreshing ? 'rotating' : ''}>↻</span> Atualizar
        </button>
      </div>

      <section className="activityBody" aria-live="polite">
        {error && <p className="pageError">{error}</p>}
        {notice && <p className="manageFeedback manageSuccess">{notice}</p>}
        {loading && !result ? <div className="activityLoading"><span className="spinner" /><p>Carregando histórico…</p></div> :
          items.length === 0 ? <div className="activityEmpty"><span>○</span><h3>Nenhum registro encontrado</h3>
            <p>{view === 'events' ? 'Os próximos eventos desta instância aparecerão aqui.' : view === 'webhooks' ? 'As próximas tentativas de webhook aparecerão aqui.' : 'Os envios assíncronos desta instância aparecerão aqui.'}</p></div> :
          <div className="activityList">
            {view === 'events' ? (items as ActivityEvent[]).map((item) => <article className="activityItem" key={item.id}>
              <span className={`activityIcon ${item.event.startsWith('message.') ? 'message' : 'system'}`}>{item.event.startsWith('message.') ? '↗' : '•'}</span>
              <div className="activityContent"><div className="activityItemHead"><strong>{eventLabels[item.event] ?? item.event}</strong><time>{dateTime(item.timestamp)}</time></div>
                <p>{eventDescription(item)}</p><div className="activityMeta"><code>{item.event}</code><span>{item.id}</span></div>
                <details><summary>Ver payload</summary><pre>{JSON.stringify(item.data, null, 2)}</pre></details>
              </div>
            </article>) : view === 'webhooks' ? (items as WebhookDelivery[]).map((item) => <article className="activityItem deliveryItem" key={item.id}>
              <span className={`activityIcon ${item.status}`}>{item.status === 'delivered' ? '✓' : '!'}</span>
              <div className="activityContent"><div className="activityItemHead"><strong>{item.status === 'delivered' ? 'Webhook entregue' : 'Falha no webhook'}</strong><time>{dateTime(item.createdAt)}</time></div>
                <p className="deliveryURL">{item.url}</p><div className="deliveryStats"><span>HTTP <strong>{item.httpStatus || '—'}</strong></span><span>Tentativa <strong>{item.attempt}</strong></span><span>Tempo <strong>{item.durationMs} ms</strong></span>{item.manual && <span className="manualTag">manual</span>}</div>
                {item.error && <p className="deliveryError">{item.error}</p>}
                <div className="activityMeta"><code>{item.event}</code><span>{item.eventId}</span></div>
                {canOperate && item.status === 'failed' && <button className="secondaryButton retryButton" type="button" disabled={retrying !== null}
                  aria-busy={retrying === item.id} onClick={() => void retryWebhook(item)}>{retrying === item.id ? 'Reenviando…' : 'Reenviar webhook'}</button>}
              </div>
            </article>) : (items as MessageJob[]).map((item) => <article className="activityItem queueItem" key={item.id}>
              <span className={`activityIcon job-${item.status}`}>{item.status === 'sent' ? '✓' : item.status === 'failed' ? '!' : item.status === 'canceled' ? '×' : '↻'}</span>
              <div className="activityContent"><div className="activityItemHead"><strong>{jobStatusLabels[item.status]} · {jobKindLabels[item.kind]}</strong><time>{dateTime(item.createdAt)}</time></div>
                <p>{jobDescription(item)}</p><div className="deliveryStats"><span>Destino <strong>{item.recipient}</strong></span><span>Tentativas <strong>{item.attempt}/{item.maxAttempts}</strong></span>
                  {item.payload.fileSize ? <span>Arquivo <strong>{fileSize(item.payload.fileSize)}</strong></span> : null}</div>
                {item.status === 'queued' && <p className="queueSchedule">Programada para {dateTime(item.scheduledAt)}</p>}
                {item.status === 'retrying' && item.nextAttemptAt && <p className="queueSchedule">Nova tentativa em {dateTime(item.nextAttemptAt)}</p>}
                {item.lastError && <p className="deliveryError">{item.lastError}</p>}
                <div className="activityMeta"><code>{item.kind}</code><span>{item.id}</span></div>
                <div className="queueActions">
                  {canOperate && (item.status === 'queued' || item.status === 'retrying') && <button className="secondaryButton retryButton" type="button" disabled={busyJob !== null}
                    aria-busy={busyJob === item.id} onClick={() => void mutateJob(item, 'cancel')}>{busyJob === item.id ? 'Cancelando…' : 'Cancelar envio'}</button>}
                  {canOperate && item.status === 'failed' && <button className="secondaryButton retryButton" type="button" disabled={busyJob !== null}
                    aria-busy={busyJob === item.id} onClick={() => void mutateJob(item, 'retry')}>{busyJob === item.id ? 'Reenviando…' : 'Tentar novamente'}</button>}
                </div>
              </div>
            </article>)}
          </div>}
      </section>

      <footer className="activityFooter"><span>{result ? `${result.total} ${result.total === 1 ? 'registro' : 'registros'}` : '—'}</span>
        <div><button className="secondaryButton" type="button" disabled={!result || page <= 1 || loading} onClick={() => setPage((value) => value - 1)}>Anterior</button>
          <small>Página {result?.page ?? page} de {Math.max(result?.totalPages ?? 1, 1)}</small>
          <button className="secondaryButton" type="button" disabled={!result || page >= result.totalPages || loading} onClick={() => setPage((value) => value + 1)}>Próxima</button></div>
      </footer>
    </dialog>
  )
}
