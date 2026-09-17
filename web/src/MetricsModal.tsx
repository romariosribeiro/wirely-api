import { useEffect, useRef, useState } from 'react'

import { request, statusLabel, type MetricsSnapshot } from './api'

type MetricsRange = MetricsSnapshot['range']

const ranges: Array<{ value: MetricsRange; label: string }> = [
  { value: '24h', label: '24 horas' },
  { value: '7d', label: '7 dias' },
  { value: '30d', label: '30 dias' },
]

function number(value: number) {
  return new Intl.NumberFormat('pt-BR', { maximumFractionDigits: 1 }).format(value)
}

function uptime(seconds: number) {
  if (seconds < 60) return `${seconds}s`
  if (seconds < 3600) return `${Math.floor(seconds / 60)}min`
  if (seconds < 86400) return `${Math.floor(seconds / 3600)}h ${Math.floor((seconds % 3600) / 60)}min`
  return `${Math.floor(seconds / 86400)}d ${Math.floor((seconds % 86400) / 3600)}h`
}

export function MetricsModal({ onClose }: { onClose: () => void }) {
  const dialog = useRef<HTMLDialogElement>(null)
  const [range, setRange] = useState<MetricsRange>('24h')
  const [metrics, setMetrics] = useState<MetricsSnapshot | null>(null)
  const [loading, setLoading] = useState(true)
  const [refreshing, setRefreshing] = useState(false)
  const [error, setError] = useState('')

  async function refresh(replace = false) {
    if (replace) setLoading(true)
    else setRefreshing(true)
    setError('')
    try {
      setMetrics(await request<MetricsSnapshot>(`/api/metrics?range=${range}`))
    } catch (reason) {
      setError(reason instanceof Error ? reason.message : 'Falha ao carregar métricas')
    } finally {
      setLoading(false)
      setRefreshing(false)
    }
  }

  useEffect(() => {
    const element = dialog.current!
    const previousFocus = document.activeElement as HTMLElement | null
    element.showModal()
    return () => { element.close(); previousFocus?.focus() }
  }, [])

  useEffect(() => {
    let active = true
    const controller = new AbortController()
    const fetchMetrics = async (background = false) => {
      if (background && active) setRefreshing(true)
      try {
        const value = await request<MetricsSnapshot>(`/api/metrics?range=${range}`, { signal: controller.signal })
        if (active) { setMetrics(value); setError('') }
      } catch (reason) {
        if (active) setError(reason instanceof Error ? reason.message : 'Falha ao carregar métricas')
      } finally {
        if (active) { setLoading(false); setRefreshing(false) }
      }
    }
    void fetchMetrics()
    const timer = window.setInterval(() => void fetchMetrics(true), 15000)
    return () => {
      active = false
      controller.abort()
      window.clearInterval(timer)
    }
  }, [range])

  return (
    <dialog ref={dialog} className="metricsModal" aria-labelledby="metrics-title"
      onCancel={(event) => { event.preventDefault(); onClose() }}>
      <header className="manageHeader metricsHeader">
        <div>
          <p className="eyebrow">OBSERVABILIDADE</p>
          <h2 id="metrics-title">Métricas operacionais</h2>
          <span className="metricsTimestamp">
            {metrics ? `Atualizado às ${new Date(metrics.generatedAt).toLocaleTimeString('pt-BR', { hour: '2-digit', minute: '2-digit', second: '2-digit' })}` : 'Visão consolidada do serviço'}
          </span>
        </div>
        <button className="closeButton" type="button" aria-label="Fechar métricas" onClick={onClose}>×</button>
      </header>

      <div className="metricsToolbar">
        <div className="metricsRanges" role="group" aria-label="Intervalo das métricas">
          {ranges.map((item) => (
            <button key={item.value} type="button" aria-pressed={range === item.value}
              onClick={() => { setRange(item.value); setMetrics(null); setLoading(true) }}>{item.label}</button>
          ))}
        </div>
        <button className="secondaryButton metricsRefresh" type="button" disabled={loading || refreshing}
          onClick={() => void refresh()}>{refreshing ? 'Atualizando…' : 'Atualizar'}</button>
      </div>

      <div className="metricsBody">
        {loading && !metrics ? (
          <div className="metricsLoading" role="status"><span className="spinner" /><p>Calculando métricas…</p></div>
        ) : error && !metrics ? (
          <div className="metricsEmpty"><strong>Não foi possível carregar</strong><p>{error}</p>
            <button className="secondaryButton" type="button" onClick={() => void refresh(true)}>Tentar novamente</button></div>
        ) : metrics && (
          <>
            {error && <p className="metricsNotice">{error}</p>}
            <section className="metricsCards" aria-label="Indicadores principais">
              <article><span>Mensagens</span><strong>{number(metrics.messages.total)}</strong>
                <small><b>{number(metrics.messages.received)}</b> recebidas · <b>{number(metrics.messages.sent)}</b> enviadas</small></article>
              <article><span>Fila pendente</span><strong className={metrics.queue.pending > 0 ? 'metricWarn' : ''}>{number(metrics.queue.pending)}</strong>
                <small>{number(metrics.queue.failed)} falhas no período</small></article>
              <article><span>Webhooks entregues</span><strong>{number(metrics.webhooks.successRate)}%</strong>
                <small>{number(metrics.webhooks.delivered)} de {number(metrics.webhooks.total)} tentativas</small>
                <i><span style={{ width: `${Math.min(100, metrics.webhooks.successRate)}%` }} /></i></article>
              <article><span>Instâncias online</span><strong>{metrics.instances.connected}/{metrics.instances.total}</strong>
                <small>uptime do serviço: {uptime(metrics.uptimeSeconds)}</small></article>
            </section>

            <section className="metricsDetail" aria-labelledby="metrics-detail-title">
              <div className="metricsSectionTitle"><div><h3 id="metrics-detail-title">Desempenho por instância</h3>
                <p>Mensagens, fila e webhooks no intervalo selecionado.</p></div>
                <span>{metrics.byInstance.length}</span></div>
              {metrics.byInstance.length === 0 ? (
                <div className="metricsEmpty compact"><strong>Nenhuma instância</strong><p>Crie uma instância para começar a acompanhar a operação.</p></div>
              ) : (
                <div className="metricsTableWrap">
                  <table className="metricsTable">
                    <thead><tr><th>Instância</th><th>Mensagens</th><th>Fila</th><th>Webhooks</th></tr></thead>
                    <tbody>{metrics.byInstance.map((item) => (
                      <tr key={item.id}>
                        <td><span className={`metricsState metricsState-${item.status}`} /><div><strong>{item.name}</strong><small>{statusLabel(item.status)}</small></div></td>
                        <td><strong>{number(item.messages.total)}</strong><small>{item.messages.received} recebidas · {item.messages.sent} enviadas</small></td>
                        <td><strong>{number(item.queue.pending)}</strong><small>{item.queue.failed} falhas · {item.queue.sent} concluídas</small></td>
                        <td><strong>{number(item.webhooks.successRate)}%</strong><small>{item.webhooks.total} tentativas · {number(item.webhooks.averageTimeMs)} ms</small></td>
                      </tr>
                    ))}</tbody>
                  </table>
                </div>
              )}
            </section>
          </>
        )}
      </div>
      <footer className="manageFooter metricsFooter">
        <span>Atualização automática a cada 15 segundos</span>
        <button className="secondaryButton" type="button" onClick={onClose}>Fechar</button>
      </footer>
    </dialog>
  )
}
