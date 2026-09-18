import { useCallback, useEffect, useRef, useState } from 'react'

import { request, type UpdateStatus } from './api'

type Props = {
  initial: UpdateStatus
  onClose: () => void
  onChanged: (status: UpdateStatus) => void
}

function version(value?: string) {
  return value ? `v${value.replace(/^v/, '')}` : '—'
}

export function UpdateModal({ initial, onClose, onChanged }: Props) {
  const dialog = useRef<HTMLDialogElement>(null)
  const [status, setStatus] = useState(initial)
  const [loading, setLoading] = useState(false)
  const [applying, setApplying] = useState(false)
  const [error, setError] = useState('')
  const [notice, setNotice] = useState('')

  const check = useCallback(async (silent = false) => {
    setLoading(true); setError(''); setNotice('')
    try {
      const value = await request<UpdateStatus>('/api/system/update?refresh=1')
      setStatus(value); onChanged(value)
      if (!silent) setNotice(value.updateAvailable ? 'Uma nova versão está disponível.' : 'Você está usando a versão mais recente.')
    } catch (reason) {
      setError(reason instanceof Error ? reason.message : 'Não foi possível verificar atualizações.')
    } finally { setLoading(false) }
  }, [onChanged])

  useEffect(() => {
    const element = dialog.current!
    const previousFocus = document.activeElement as HTMLElement | null
    element.showModal()
    const refreshTimer = window.setTimeout(() => void check(true), 0)
    return () => {
      window.clearTimeout(refreshTimer)
      element.close(); previousFocus?.focus()
    }
  }, [check])

  async function apply() {
    if (!window.confirm(`Criar um backup e atualizar para ${version(status.latestVersion)}? O Wirely reiniciará automaticamente.`)) return
    setApplying(true); setError(''); setNotice('')
    try {
      await request('/api/system/update', { method: 'POST' })
      setNotice('Backup concluído e atualização validada. O Wirely está reiniciando…')
      window.setTimeout(() => window.location.reload(), 7000)
    } catch (reason) {
      setError(reason instanceof Error ? reason.message : 'Não foi possível aplicar a atualização.')
      setApplying(false)
    }
  }

  return <dialog ref={dialog} className="updateModal" aria-labelledby="update-title"
    onCancel={(event) => { event.preventDefault(); if (!applying) onClose() }}>
    <header className="manageHeader updateHeader">
      <div><p className="eyebrow">ATUALIZAÇÃO SEGURA</p><h2 id="update-title">Atualizações da Wirely</h2>
        <span>Release oficial · checksum SHA-256 · rollback do binário</span></div>
      <button className="closeButton" type="button" aria-label="Fechar atualização" disabled={applying} onClick={onClose}>×</button>
    </header>
    <div className="updateBody">
      {error && <p className="pageError" role="alert">{error}</p>}
      {notice && <p className="manageFeedback manageSuccess" role="status">{notice}</p>}
      <section className={`updateHero ${status.updateAvailable ? 'available' : ''}`}>
        <div className="updateIcon" aria-hidden="true">{status.updateAvailable ? '↓' : '✓'}</div>
        <div><span>{status.updateAvailable ? 'Atualização disponível' : 'Wirely atualizada'}</span>
          <strong>{status.updateAvailable ? `${version(status.currentVersion)} → ${version(status.latestVersion)}` : version(status.currentVersion)}</strong>
          <small>{status.message || (status.updateAvailable ? 'Pronta para instalação segura.' : 'Nenhuma ação necessária.')}</small></div>
      </section>

      {status.updateAvailable && <section className="updateNotes">
        <div className="updateNotesHeading"><div><span>O QUE MUDOU</span><h3>{status.title || version(status.latestVersion)}</h3></div>
          {status.publishedAt && <time dateTime={status.publishedAt}>{new Date(status.publishedAt).toLocaleDateString('pt-BR')}</time>}</div>
        <div className="updateNotesText">{status.notes || 'Esta release não possui notas detalhadas.'}</div>
        {status.releaseUrl && <a href={status.releaseUrl} target="_blank" rel="noreferrer">Ver release no GitHub ↗</a>}
      </section>}

      <section className="updateSafety">
        <div><span>1</span><p><strong>Backup automático</strong><small>Banco, sessões e configurações são salvos antes de atualizar.</small></p></div>
        <div><span>2</span><p><strong>Pacote verificado</strong><small>A instalação só continua se o SHA-256 oficial corresponder.</small></p></div>
        <div><span>3</span><p><strong>Retorno seguro</strong><small>O binário anterior permanece disponível como rollback.</small></p></div>
      </section>
    </div>
    <footer className="manageFooter updateFooter">
      <button className="secondaryButton" type="button" disabled={loading || applying} onClick={() => void check()}>
        {loading ? 'Verificando…' : 'Verificar novamente'}</button>
      <button className="primaryButton" type="button" disabled={!status.canApply || loading || applying}
        aria-busy={applying} onClick={() => void apply()}>
        {applying ? 'Preparando atualização…' : 'Fazer backup e aplicar atualização'}</button>
    </footer>
  </dialog>
}
