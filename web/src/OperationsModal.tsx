import { useCallback, useEffect, useRef, useState } from 'react'

import { can, request, type Alert, type AuditEntry, type BackupStatus, type Page, type User } from './api'

type View = 'alerts' | 'backups' | 'audit'

const actionLabels: Record<string, string> = {
  'auth.login': 'Acesso ao painel', 'auth.password.change': 'Senha alterada',
  'user.create': 'Usuário criado', 'user.update': 'Usuário atualizado',
  'user.password.reset': 'Senha redefinida', 'user.delete': 'Usuário excluído',
  'instance.create': 'Instância criada', 'instance.connect': 'Instância conectada',
  'instance.disconnect': 'Instância desconectada', 'instance.token.rotate': 'Token renovado',
  'instance.webhook.update': 'Webhook atualizado', 'instance.proxy.update': 'Proxy atualizado',
  'instance.proxy.delete': 'Proxy removido', 'instance.delete': 'Instância excluída',
  'webhook.retry': 'Webhook reenviado', 'chat.message.send': 'Mensagem enviada',
  'chat.read': 'Conversa marcada como lida', 'queue.cancel': 'Envio cancelado',
  'queue.retry': 'Envio reenviado', 'backup.create': 'Backup criado',
  'backup.restore': 'Restauração preparada', 'backup.delete': 'Backup excluído',
  'system.update': 'Wirely atualizada',
}

const reasonLabels: Record<string, string> = {
  manual: 'Manual', automatic: 'Automático', 'pre-restore': 'Segurança pré-restauração',
  'pre-update': 'Segurança pré-atualização',
}

function dateTime(value: string) {
  return new Intl.DateTimeFormat('pt-BR', { dateStyle: 'short', timeStyle: 'medium' }).format(new Date(value))
}

function fileSize(value: number) {
  if (value >= 1024 * 1024 * 1024) return `${(value / 1024 / 1024 / 1024).toFixed(1)} GB`
  if (value >= 1024 * 1024) return `${(value / 1024 / 1024).toFixed(1)} MB`
  return `${Math.max(1, Math.ceil(value / 1024))} KB`
}

export function OperationsModal({ user, onClose }: { user: User; onClose: () => void }) {
  const dialog = useRef<HTMLDialogElement>(null)
  const canAudit = can(user, 'admin')
  const canBackup = user.role === 'owner'
  const [view, setView] = useState<View>('alerts')
  const [alerts, setAlerts] = useState<Alert[]>([])
  const [backups, setBackups] = useState<BackupStatus | null>(null)
  const [audit, setAudit] = useState<Page<AuditEntry> | null>(null)
  const [loading, setLoading] = useState(true)
  const [busy, setBusy] = useState('')
  const [error, setError] = useState('')
  const [notice, setNotice] = useState('')

  useEffect(() => {
    const element = dialog.current!
    const previousFocus = document.activeElement as HTMLElement | null
    element.showModal()
    return () => { element.close(); previousFocus?.focus() }
  }, [])

  const load = useCallback(async () => {
    setLoading(true); setError('')
    try {
      if (view === 'alerts') {
        const response = await request<{ data: Alert[] }>('/api/alerts')
        setAlerts(response.data)
      } else if (view === 'backups') {
        setBackups(await request<BackupStatus>('/api/backups'))
      } else {
        setAudit(await request<Page<AuditEntry>>('/api/audit?page=1&pageSize=50'))
      }
    } catch (reason) {
      setError(reason instanceof Error ? reason.message : 'Não foi possível carregar as operações.')
    } finally { setLoading(false) }
  }, [view])

  useEffect(() => {
    const timer = window.setTimeout(() => void load(), 0)
    return () => window.clearTimeout(timer)
  }, [load])

  async function createBackup() {
    setBusy('create'); setError(''); setNotice('')
    try {
      await request('/api/backups', { method: 'POST' })
      setNotice('Backup criado e validado com sucesso.')
      await load()
    } catch (reason) {
      setError(reason instanceof Error ? reason.message : 'Não foi possível criar o backup.')
    } finally { setBusy('') }
  }

  async function restoreBackup(id: string) {
    if (!window.confirm('Restaurar este backup? O Wirely criará um backup de segurança e reiniciará automaticamente.')) return
    setBusy(`restore:${id}`); setError(''); setNotice('')
    try {
      await request(`/api/backups/${id}/restore`, { method: 'POST' })
      setNotice('Restauração preparada. O Wirely está reiniciando; recarregue a página em alguns segundos.')
    } catch (reason) {
      setError(reason instanceof Error ? reason.message : 'Não foi possível preparar a restauração.')
      setBusy('')
    }
  }

  async function deleteBackup(id: string) {
    if (!window.confirm('Excluir este arquivo de backup?')) return
    setBusy(`delete:${id}`); setError(''); setNotice('')
    try {
      await request(`/api/backups/${id}`, { method: 'DELETE' })
      setNotice('Backup excluído.')
      await load()
    } catch (reason) {
      setError(reason instanceof Error ? reason.message : 'Não foi possível excluir o backup.')
    } finally { setBusy('') }
  }

  return (
    <dialog ref={dialog} className="operationsModal" aria-labelledby="operations-title"
      onCancel={(event) => { event.preventDefault(); if (!busy) onClose() }}>
      <header className="manageHeader">
        <div><p className="eyebrow">OPERAÇÃO E SEGURANÇA</p><h2 id="operations-title">Central operacional</h2>
          <span className="operationsSubtitle">Alertas, recuperação e trilha administrativa</span></div>
        <button className="closeButton" type="button" aria-label="Fechar operações" disabled={Boolean(busy)} onClick={onClose}>×</button>
      </header>

      <nav className="operationsTabs" aria-label="Seções operacionais">
        <button type="button" aria-pressed={view === 'alerts'} onClick={() => setView('alerts')}>Alertas</button>
        {canBackup && <button type="button" aria-pressed={view === 'backups'} onClick={() => setView('backups')}>Backups</button>}
        {canAudit && <button type="button" aria-pressed={view === 'audit'} onClick={() => setView('audit')}>Auditoria</button>}
      </nav>

      <div className="operationsBody">
        {error && <p className="pageError">{error}</p>}
        {notice && <p className="manageFeedback manageSuccess">{notice}</p>}
        {loading ? <div className="operationsLoading"><span className="spinner" /><p>Carregando dados operacionais…</p></div> : <>
          {view === 'alerts' && <section>
            <div className="operationsHeading"><div><h3>Alertas ativos</h3><p>Condições calculadas com os dados das últimas 24 horas.</p></div><span>{alerts.length}</span></div>
            {alerts.length === 0 ? <div className="operationsEmpty"><span>✓</span><strong>Operação saudável</strong><p>Nenhum alerta ativo neste momento.</p></div> :
              <div className="alertList">{alerts.map((item) => <article className={`alertItem ${item.severity}`} key={item.id}>
                <i /><div><strong>{item.title}</strong><p>{item.message}</p></div><span>{item.severity === 'critical' ? 'Crítico' : 'Atenção'}</span>
              </article>)}</div>}
          </section>}

          {view === 'backups' && backups && <section>
            <div className="operationsHeading"><div><h3>Backup e restauração</h3>
              <p>{backups.automatic ? `Automático a cada ${backups.intervalHours}h · retenção de ${backups.retention} arquivos` : 'Backup automático desativado'}</p></div>
              <button className="primaryButton" type="button" disabled={Boolean(busy)} onClick={() => void createBackup()}>{busy === 'create' ? 'Criando…' : 'Criar backup'}</button></div>
            {backups.nextBackupAt && <p className="backupNext">Próximo backup: {dateTime(backups.nextBackupAt)}</p>}
            {backups.data.length === 0 ? <div className="operationsEmpty"><strong>Nenhum backup disponível</strong><p>Crie o primeiro ponto de restauração.</p></div> :
              <div className="backupList">{backups.data.map((item) => <article key={item.id}>
                <div><strong>{dateTime(item.createdAt)}</strong><small>{reasonLabels[item.reason] ?? item.reason} · {fileSize(item.size)}</small></div>
                <div><a className="secondaryButton" href={`/api/backups/${item.id}/download`}>Baixar</a>
                  <button className="secondaryButton" type="button" disabled={Boolean(busy)} onClick={() => void restoreBackup(item.id)}>
                    {busy === `restore:${item.id}` ? 'Reiniciando…' : 'Restaurar'}</button>
                  <button className="dangerButton" type="button" disabled={Boolean(busy)} onClick={() => void deleteBackup(item.id)}>
                    {busy === `delete:${item.id}` ? 'Excluindo…' : 'Excluir'}</button></div>
              </article>)}</div>}
          </section>}

          {view === 'audit' && audit && <section>
            <div className="operationsHeading"><div><h3>Auditoria administrativa</h3><p>Ações registradas sem senhas, tokens ou conteúdo das requisições.</p></div><span>{audit.total}</span></div>
            {audit.data.length === 0 ? <div className="operationsEmpty"><strong>Nenhum evento registrado</strong></div> :
              <div className="auditList">{audit.data.map((item) => <article key={item.id}>
                <span className={item.status < 400 ? 'auditOK' : 'auditFailed'}>{item.status}</span>
                <div><strong>{actionLabels[item.action] ?? item.action}</strong><small>{item.username || 'Sistema'} · {dateTime(item.createdAt)}</small></div>
                <code>{item.targetId || item.sourceIp || '—'}</code>
              </article>)}</div>}
          </section>}
        </>}
      </div>
      <footer className="manageFooter"><span className="operationsFooterNote">Prometheus disponível em /metrics</span>
        <button className="secondaryButton" type="button" disabled={Boolean(busy)} onClick={onClose}>Fechar</button></footer>
    </dialog>
  )
}
