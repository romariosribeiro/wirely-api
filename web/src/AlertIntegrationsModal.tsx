import { FormEvent, useEffect, useRef, useState } from 'react'

import { request, type AlertIntegrations, type AlertIntegrationsUpdate, type Instance } from './api'

type Draft = AlertIntegrations & {
  telegram: AlertIntegrations['telegram'] & { token: string; clearToken: boolean }
  smtp: AlertIntegrations['smtp'] & { password: string; clearPassword: boolean }
}

function toDraft(value: AlertIntegrations): Draft {
  return {
    telegram: { ...value.telegram, instanceIds: value.telegram.instanceIds ?? [], token: '', clearToken: false },
    smtp: { ...value.smtp, instanceIds: value.smtp.instanceIds ?? [], password: '', clearPassword: false },
  }
}

export function AlertIntegrationsModal({ instances, onClose }: { instances: Instance[]; onClose: () => void }) {
  const dialog = useRef<HTMLDialogElement>(null)
  const [draft, setDraft] = useState<Draft | null>(null)
  const [busy, setBusy] = useState('load')
  const [error, setError] = useState('')
  const [notice, setNotice] = useState('')

  useEffect(() => {
    const element = dialog.current!
    const previousFocus = document.activeElement as HTMLElement | null
    element.showModal()
    request<AlertIntegrations>('/api/alert-integrations').then((value) => setDraft(toDraft(value)))
      .catch((reason) => setError(reason instanceof Error ? reason.message : 'Falha ao carregar integrações.')).finally(() => setBusy(''))
    return () => { element.close(); previousFocus?.focus() }
  }, [])

  function toggleInstance(channel: 'telegram' | 'smtp', id: string) {
    setDraft((current) => {
      if (!current) return current
      const ids = current[channel].instanceIds.includes(id) ? current[channel].instanceIds.filter((value) => value !== id) : [...current[channel].instanceIds, id]
      return { ...current, [channel]: { ...current[channel], instanceIds: ids } }
    })
  }

  async function save(event: FormEvent) {
    event.preventDefault()
    if (!draft) return
    setBusy('save'); setError(''); setNotice('')
    try {
      const payload: AlertIntegrationsUpdate = {
        telegram: {
          enabled: draft.telegram.enabled,
          token: draft.telegram.token,
          clearToken: draft.telegram.clearToken,
          chatId: draft.telegram.chatId,
          instanceIds: draft.telegram.instanceIds,
        },
        smtp: {
          enabled: draft.smtp.enabled,
          host: draft.smtp.host,
          port: draft.smtp.port,
          security: draft.smtp.security,
          username: draft.smtp.username,
          password: draft.smtp.password,
          clearPassword: draft.smtp.clearPassword,
          from: draft.smtp.from,
          recipient: draft.smtp.recipient,
          instanceIds: draft.smtp.instanceIds,
        },
      }
      const saved = await request<AlertIntegrations>('/api/alert-integrations', { method: 'PUT', body: JSON.stringify(payload) })
      setDraft(toDraft(saved)); setNotice('Integrações salvas com segurança.')
    } catch (reason) { setError(reason instanceof Error ? reason.message : 'Falha ao salvar integrações.') }
    finally { setBusy('') }
  }

  async function test(channel: 'telegram' | 'smtp') {
    setBusy(`test:${channel}`); setError(''); setNotice('')
    try {
      await request(`/api/alert-integrations/test/${channel}`, { method: 'POST' })
      setNotice(channel === 'telegram' ? 'Mensagem de teste enviada pelo Telegram.' : 'E-mail de teste enviado pelo SMTP.')
    } catch (reason) { setError(reason instanceof Error ? reason.message : 'Falha ao enviar o teste.') }
    finally { setBusy('') }
  }

  const locked = Boolean(busy)
  return <dialog ref={dialog} className="alertIntegrationsModal" aria-labelledby="alert-integrations-title"
    onCancel={(event) => { event.preventDefault(); if (!locked) onClose() }}>
    <header className="manageHeader"><div><p className="eyebrow">NOTIFICAÇÕES</p><h2 id="alert-integrations-title">Integrações de alertas</h2>
      <span className="operationsSubtitle">Receba avisos de queda e recuperação das instâncias selecionadas.</span></div>
      <button className="closeButton" type="button" aria-label="Fechar integrações" disabled={locked} onClick={onClose}>×</button></header>
    {!draft ? <div className="operationsLoading">{error ? <><p className="pageError">{error}</p><button className="secondaryButton" type="button" onClick={onClose}>Fechar</button></> : <><span className="spinner" /><p>Carregando integrações…</p></>}</div> :
      <form className="alertIntegrationsBody" onSubmit={save}>
        {error && <p className="pageError">{error}</p>}{notice && <p className="manageFeedback manageSuccess">{notice}</p>}
        <section className="alertChannel">
          <div className="alertChannelHeading"><div className="channelIcon telegramIcon">T</div><div><h3>Telegram</h3><p>Envie alertas diretamente para um chat, grupo ou canal.</p></div>
            <label className="manageSwitch"><input type="checkbox" role="switch" checked={draft.telegram.enabled} disabled={locked}
              onChange={(event) => setDraft({ ...draft, telegram: { ...draft.telegram, enabled: event.target.checked } })} /><span>{draft.telegram.enabled ? 'Ativo' : 'Desativado'}</span></label></div>
          <div className="alertFieldGrid">
            <label className="wide">Token do bot <input type="password" autoComplete="new-password" value={draft.telegram.token} disabled={locked}
              placeholder={draft.telegram.hasToken ? 'Token salvo — deixe em branco para manter' : '123456789:AA...'} onChange={(event) => setDraft({ ...draft, telegram: { ...draft.telegram, token: event.target.value, clearToken: false } })} /></label>
            <label>ID do chat <input value={draft.telegram.chatId} disabled={locked} placeholder="-1001234567890" onChange={(event) => setDraft({ ...draft, telegram: { ...draft.telegram, chatId: event.target.value } })} /></label>
          </div>
          {draft.telegram.hasToken && <label className="alertClear"><input type="checkbox" checked={draft.telegram.clearToken} disabled={locked} onChange={(event) => setDraft({ ...draft, telegram: { ...draft.telegram, clearToken: event.target.checked, token: '' } })} /> Remover token salvo</label>}
          <InstancePicker instances={instances} selected={draft.telegram.instanceIds} disabled={locked} onToggle={(id) => toggleInstance('telegram', id)} />
          <div className="alertTest"><small>Salve a configuração antes de testar.</small><button className="secondaryButton" type="button" disabled={locked || !draft.telegram.enabled || !draft.telegram.hasToken} onClick={() => void test('telegram')}>{busy === 'test:telegram' ? 'Enviando…' : 'Enviar teste'}</button></div>
        </section>
        <section className="alertChannel">
          <div className="alertChannelHeading"><div className="channelIcon smtpIcon">@</div><div><h3>SMTP</h3><p>Entregue alertas por e-mail usando seu próprio servidor.</p></div>
            <label className="manageSwitch"><input type="checkbox" role="switch" checked={draft.smtp.enabled} disabled={locked} onChange={(event) => setDraft({ ...draft, smtp: { ...draft.smtp, enabled: event.target.checked } })} /><span>{draft.smtp.enabled ? 'Ativo' : 'Desativado'}</span></label></div>
          <div className="alertFieldGrid smtpGrid">
            <label className="wide">Servidor SMTP <input value={draft.smtp.host} disabled={locked} placeholder="smtp.exemplo.com" onChange={(event) => setDraft({ ...draft, smtp: { ...draft.smtp, host: event.target.value } })} /></label>
            <label>Porta <input type="number" min="1" max="65535" value={draft.smtp.port} disabled={locked} onChange={(event) => setDraft({ ...draft, smtp: { ...draft.smtp, port: Number(event.target.value) } })} /></label>
            <label>Segurança <select value={draft.smtp.security} disabled={locked} onChange={(event) => setDraft({ ...draft, smtp: { ...draft.smtp, security: event.target.value as Draft['smtp']['security'] } })}><option value="starttls">STARTTLS</option><option value="tls">TLS implícito</option><option value="none">Sem criptografia</option></select></label>
            <label>Usuário <input value={draft.smtp.username} disabled={locked} autoComplete="username" onChange={(event) => setDraft({ ...draft, smtp: { ...draft.smtp, username: event.target.value } })} /></label>
            <label>Senha <input type="password" value={draft.smtp.password} disabled={locked} autoComplete="new-password" placeholder={draft.smtp.hasPassword ? 'Senha salva — deixe em branco para manter' : 'Senha SMTP'} onChange={(event) => setDraft({ ...draft, smtp: { ...draft.smtp, password: event.target.value, clearPassword: false } })} /></label>
            <label>Remetente <input value={draft.smtp.from} disabled={locked} placeholder="Wirely <alertas@exemplo.com>" onChange={(event) => setDraft({ ...draft, smtp: { ...draft.smtp, from: event.target.value } })} /></label>
            <label>E-mail que receberá os alertas <input type="email" value={draft.smtp.recipient} disabled={locked} placeholder="voce@exemplo.com" onChange={(event) => setDraft({ ...draft, smtp: { ...draft.smtp, recipient: event.target.value } })} /></label>
          </div>
          {draft.smtp.hasPassword && <label className="alertClear"><input type="checkbox" checked={draft.smtp.clearPassword} disabled={locked} onChange={(event) => setDraft({ ...draft, smtp: { ...draft.smtp, clearPassword: event.target.checked, password: '' } })} /> Remover senha salva</label>}
          <InstancePicker instances={instances} selected={draft.smtp.instanceIds} disabled={locked} onToggle={(id) => toggleInstance('smtp', id)} />
          <div className="alertTest"><small>Use STARTTLS (porta 587) ou TLS (porta 465) sempre que possível.</small><button className="secondaryButton" type="button" disabled={locked || !draft.smtp.enabled} onClick={() => void test('smtp')}>{busy === 'test:smtp' ? 'Enviando…' : 'Enviar teste'}</button></div>
        </section>
        <footer className="alertIntegrationsFooter"><span>Tokens e senhas são criptografados antes de serem salvos.</span><button className="secondaryButton" type="button" disabled={locked} onClick={onClose}>Cancelar</button><button className="primaryButton" type="submit" disabled={locked}>{busy === 'save' ? 'Salvando…' : 'Salvar integrações'}</button></footer>
      </form>}
  </dialog>
}

function InstancePicker({ instances, selected, disabled, onToggle }: { instances: Instance[]; selected: string[]; disabled: boolean; onToggle: (id: string) => void }) {
  const selectedIds = selected ?? []
  return <fieldset className="alertInstances"><legend>Instâncias monitoradas</legend>{instances.length === 0 ? <p>Nenhuma instância disponível.</p> : instances.map((instance) => <label key={instance.id}><input type="checkbox" checked={selectedIds.includes(instance.id)} disabled={disabled} onChange={() => onToggle(instance.id)} /><span><strong>{instance.name}</strong><small className={`badge badge-${instance.status}`}>{instance.status}</small></span></label>)}</fieldset>
}
