import { FormEvent, useEffect, useRef, useState } from 'react'
import { request, type User, type UserRole } from './api'

const roleLabels: Record<UserRole, string> = {
  owner: 'Proprietário', admin: 'Administrador', operator: 'Operador', viewer: 'Visualizador',
}

type Draft = { role: UserRole; enabled: boolean }

export function UsersModal({ onClose }: { onClose: () => void }) {
  const dialog = useRef<HTMLDialogElement>(null)
  const [users, setUsers] = useState<User[]>([])
  const [drafts, setDrafts] = useState<Record<string, Draft>>({})
  const [username, setUsername] = useState('')
  const [password, setPassword] = useState('')
  const [role, setRole] = useState<UserRole>('operator')
  const [resetUser, setResetUser] = useState<User | null>(null)
  const [resetPassword, setResetPassword] = useState('')
  const [busy, setBusy] = useState('')
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')
  const [notice, setNotice] = useState('')

  useEffect(() => {
    const element = dialog.current!
    const previousFocus = document.activeElement as HTMLElement | null
    element.showModal()
    return () => { element.close(); previousFocus?.focus() }
  }, [])

  async function load() {
    try {
      const response = await request<{ data: User[] }>('/api/v1/users')
      setUsers(response.data)
      setDrafts(Object.fromEntries(response.data.map((user) => [user.id, { role: user.role, enabled: user.enabled }])))
      setError('')
    } catch (reason) {
      setError(reason instanceof Error ? reason.message : 'Não foi possível carregar a equipe.')
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => {
    const timer = window.setTimeout(() => void load(), 0)
    return () => window.clearTimeout(timer)
  }, [])

  async function create(event: FormEvent) {
    event.preventDefault(); setBusy('create'); setError(''); setNotice('')
    try {
      await request('/api/v1/users', { method: 'POST', body: JSON.stringify({ username, password, role }) })
      setUsername(''); setPassword(''); setRole('operator'); setNotice('Usuário criado e pronto para acessar o painel.')
      await load()
    } catch (reason) {
      setError(reason instanceof Error ? reason.message : 'Não foi possível criar o usuário.')
    } finally { setBusy('') }
  }

  function changeDraft(id: string, next: Partial<Draft>) {
    setDrafts((current) => ({ ...current, [id]: { ...current[id], ...next } }))
  }

  async function save(user: User) {
    const draft = drafts[user.id]
    if (!draft) return
    setBusy(user.id); setError(''); setNotice('')
    try {
      await request(`/api/v1/users/${user.id}`, { method: 'PATCH', body: JSON.stringify(draft) })
      setNotice(`Permissões de ${user.username} atualizadas. As sessões anteriores foram encerradas.`)
      await load()
    } catch (reason) {
      setError(reason instanceof Error ? reason.message : 'Não foi possível atualizar o usuário.')
    } finally { setBusy('') }
  }

  async function reset(event: FormEvent) {
    event.preventDefault()
    if (!resetUser) return
    setBusy(`password:${resetUser.id}`); setError(''); setNotice('')
    try {
      await request(`/api/v1/users/${resetUser.id}/password`, {
        method: 'PUT', body: JSON.stringify({ password: resetPassword }),
      })
      setNotice(`Senha de ${resetUser.username} redefinida. As sessões anteriores foram encerradas.`)
      setResetUser(null); setResetPassword('')
    } catch (reason) {
      setError(reason instanceof Error ? reason.message : 'Não foi possível redefinir a senha.')
    } finally { setBusy('') }
  }

  async function remove(user: User) {
    if (!window.confirm(`Excluir o acesso de ${user.username}? Esta ação encerra todas as sessões desse usuário.`)) return
    setBusy(`delete:${user.id}`); setError(''); setNotice('')
    try {
      await request(`/api/v1/users/${user.id}`, { method: 'DELETE' })
      setNotice(`Usuário ${user.username} removido.`)
      await load()
    } catch (reason) {
      setError(reason instanceof Error ? reason.message : 'Não foi possível excluir o usuário.')
    } finally { setBusy('') }
  }

  return (
    <dialog ref={dialog} className="usersModal" aria-labelledby="users-title"
      onCancel={(event) => { event.preventDefault(); if (!busy) onClose() }}>
      <header className="manageHeader">
        <div><p className="eyebrow">CONTROLE DE ACESSO</p><h2 id="users-title">Equipe</h2>
          <span className="usersSubtitle">Papéis e sessões protegidos no servidor</span></div>
        <button className="closeButton" type="button" aria-label="Fechar equipe" disabled={Boolean(busy)} onClick={onClose}>×</button>
      </header>

      <div className="usersBody">
        <section className="usersCreate" aria-labelledby="new-user-title">
          <div className="usersSectionHeading"><div><h3 id="new-user-title">Adicionar usuário</h3>
            <p>Crie uma credencial individual. A senha não será exibida novamente.</p></div></div>
          <form onSubmit={create}>
            <label><span>Usuário</span><input aria-label="Novo usuário" value={username}
              onChange={(event) => setUsername(event.target.value)} minLength={3} maxLength={40} placeholder="ex.: suporte" required /></label>
            <label><span>Senha inicial</span><input aria-label="Senha inicial" type="password" value={password}
              onChange={(event) => setPassword(event.target.value)} minLength={12} maxLength={128}
              autoComplete="new-password" placeholder="mínimo 12 caracteres" required /></label>
            <label><span>Papel</span><select aria-label="Papel do novo usuário" value={role}
              onChange={(event) => setRole(event.target.value as UserRole)}>
              <option value="admin">Administrador</option><option value="operator">Operador</option><option value="viewer">Visualizador</option>
            </select></label>
            <button className="primaryButton" type="submit" disabled={Boolean(busy)} aria-busy={busy === 'create'}>
              {busy === 'create' ? 'Criando…' : 'Adicionar'}</button>
          </form>
        </section>

        {error && <p className="pageError usersFeedback">{error}</p>}
        {notice && <p className="manageFeedback manageSuccess usersFeedback">{notice}</p>}

        <section className="usersListSection" aria-labelledby="team-list-title">
          <div className="usersSectionHeading"><div><h3 id="team-list-title">Acessos cadastrados</h3>
            <p>Alterar papel, estado ou senha encerra as sessões abertas desse usuário.</p></div><span>{users.length}</span></div>
          {loading ? <div className="usersLoading"><span className="spinner" /><p>Carregando equipe…</p></div> :
            <div className="usersList">{users.map((user) => {
              const draft = drafts[user.id] ?? { role: user.role, enabled: user.enabled }
              const owner = user.role === 'owner'
              const changed = draft.role !== user.role || draft.enabled !== user.enabled
              return <article className={`userRow ${!draft.enabled ? 'disabled' : ''}`} key={user.id}>
                <div className="userIdentity"><span>{user.username.slice(0, 2).toUpperCase()}</span><div>
                  <strong>{user.username}</strong><small>{owner ? 'Conta principal protegida' : user.lastLoginAt ? `Último acesso ${new Date(user.lastLoginAt).toLocaleString('pt-BR')}` : 'Ainda não acessou'}</small>
                </div></div>
                <div className="userControls">
                  <label><span className="visuallyHidden">Papel de {user.username}</span><select aria-label={`Papel de ${user.username}`}
                    value={draft.role} disabled={owner || Boolean(busy)} onChange={(event) => changeDraft(user.id, { role: event.target.value as UserRole })}>
                    {owner && <option value="owner">Proprietário</option>}<option value="admin">Administrador</option>
                    <option value="operator">Operador</option><option value="viewer">Visualizador</option>
                  </select></label>
                  <label className="manageSwitch userEnabled"><input type="checkbox" aria-label={`Acesso ativo de ${user.username}`}
                    checked={draft.enabled} disabled={owner || Boolean(busy)} onChange={(event) => changeDraft(user.id, { enabled: event.target.checked })} />
                    <span>{draft.enabled ? 'Ativo' : 'Bloqueado'}</span></label>
                  {!owner && <div className="userActions">
                    <button className="secondaryButton" type="button" disabled={!changed || Boolean(busy)} onClick={() => void save(user)}>
                      {busy === user.id ? 'Salvando…' : 'Salvar'}</button>
                    <button className="textButton" type="button" disabled={Boolean(busy)} onClick={() => { setResetUser(user); setResetPassword(''); setNotice('') }}>Redefinir senha</button>
                    <button className="dangerButton" type="button" disabled={Boolean(busy)} onClick={() => void remove(user)}>Excluir</button>
                  </div>}
                  {owner && <span className="roleBadge">{roleLabels.owner}</span>}
                </div>
              </article>
            })}</div>}
        </section>

        {resetUser && <section className="userReset" aria-labelledby="reset-user-title">
          <div><h3 id="reset-user-title">Redefinir senha · {resetUser.username}</h3><p>A sessão atual desse usuário será encerrada imediatamente.</p></div>
          <form onSubmit={reset}><input aria-label={`Nova senha de ${resetUser.username}`} type="password" value={resetPassword}
            onChange={(event) => setResetPassword(event.target.value)} minLength={12} maxLength={128} autoComplete="new-password" autoFocus required />
            <button className="secondaryButton" type="button" disabled={Boolean(busy)} onClick={() => setResetUser(null)}>Cancelar</button>
            <button className="primaryButton" type="submit" disabled={Boolean(busy)}>{busy.startsWith('password:') ? 'Salvando…' : 'Salvar nova senha'}</button>
          </form>
        </section>}
      </div>
      <footer className="manageFooter usersFooter"><span>Proprietário · Administrador · Operador · Visualizador</span>
        <button className="secondaryButton" type="button" disabled={Boolean(busy)} onClick={onClose}>Fechar</button></footer>
    </dialog>
  )
}
