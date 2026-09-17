import { useCallback, useEffect, useMemo, useRef, useState, type FormEvent, type KeyboardEvent } from 'react'
import { request, type ChatMessage, type ChatSummary, type Contact, type Instance, type Page } from './api'

type InboxView = 'chats' | 'contacts'
type Selection = { chat: string; name: string; isGroup: boolean }

function chatName(chat: string, name: string, isGroup: boolean) {
  if (name.trim()) return name
  const identifier = chat.split('@')[0]
  return isGroup ? `Grupo ${identifier}` : `+${identifier}`
}

function initials(name: string) {
  const parts = name.trim().split(/\s+/).filter(Boolean)
  if (parts.length === 0) return '?'
  const compact = parts.length > 1 ? (parts.at(0)?.charAt(0) ?? '') + (parts.at(-1)?.charAt(0) ?? '') : name.slice(0, 2)
  return compact.toUpperCase()
}

function preview(message: ChatMessage) {
  if (message.event === 'message.deleted') return 'Mensagem apagada'
  if (message.event === 'message.reaction') return `Reação ${String(message.data.reaction ?? '')}`.trim()
  if (message.text) return message.text
  const labels: Record<string, string> = { image: 'Imagem', audio: 'Áudio', video: 'Vídeo', document: 'Documento', sticker: 'Figurinha' }
  return (message.type ? labels[message.type] : undefined) ?? 'Mensagem'
}

function shortTime(value: string) {
  const date = new Date(value)
  const today = new Date()
  if (date.toDateString() === today.toDateString()) return date.toLocaleTimeString('pt-BR', { hour: '2-digit', minute: '2-digit' })
  return date.toLocaleDateString('pt-BR', { day: '2-digit', month: '2-digit' })
}

function fullTime(value: string) {
  return new Date(value).toLocaleString('pt-BR', { dateStyle: 'short', timeStyle: 'short' })
}

export function InboxModal({ instance, canSend, onClose }: { instance: Instance; canSend: boolean; onClose: () => void }) {
  const dialog = useRef<HTMLDialogElement>(null)
  const messageEnd = useRef<HTMLDivElement>(null)
  const [view, setView] = useState<InboxView>('chats')
  const [search, setSearch] = useState('')
  const [chats, setChats] = useState<Page<ChatSummary> | null>(null)
  const [contacts, setContacts] = useState<Page<Contact> | null>(null)
  const [selected, setSelected] = useState<Selection | null>(null)
  const [messages, setMessages] = useState<ChatMessage[]>([])
  const [messagePage, setMessagePage] = useState(1)
  const [messagePages, setMessagePages] = useState(1)
  const [draft, setDraft] = useState('')
  const [loadingList, setLoadingList] = useState(true)
  const [loadingMessages, setLoadingMessages] = useState(false)
  const [loadingOlder, setLoadingOlder] = useState(false)
  const [sending, setSending] = useState(false)
  const [error, setError] = useState('')

  useEffect(() => {
    const element = dialog.current!
    const previousFocus = document.activeElement as HTMLElement | null
    element.showModal()
    return () => { element.close(); previousFocus?.focus() }
  }, [])

  const loadList = useCallback(async (quiet = false, signal?: AbortSignal) => {
    if (!quiet) setLoadingList(true)
    const query = new URLSearchParams({ page: '1', pageSize: '100' })
    if (search.trim()) query.set('search', search.trim())
    try {
      if (view === 'chats') setChats(await request<Page<ChatSummary>>(`/api/v1/instances/${instance.id}/chats?${query}`, { signal }))
      else setContacts(await request<Page<Contact>>(`/api/v1/instances/${instance.id}/contacts?${query}`, { signal }))
      if (!signal?.aborted) setError('')
    } catch (reason) {
      if (!signal?.aborted) setError(reason instanceof Error ? reason.message : 'Falha ao carregar conversas.')
    } finally { if (!signal?.aborted && !quiet) setLoadingList(false) }
  }, [instance.id, search, view])

  useEffect(() => {
    const controller = new AbortController()
    const initial = window.setTimeout(() => void loadList(false, controller.signal), 180)
    const timer = window.setInterval(() => void loadList(true, controller.signal), 6000)
    return () => { controller.abort(); window.clearTimeout(initial); window.clearInterval(timer) }
  }, [loadList])

  const loadMessages = useCallback(async (selection: Selection, page = 1, append = false, signal?: AbortSignal, refresh = false) => {
    if (!refresh) {
      if (append) setLoadingOlder(true); else setLoadingMessages(true)
    }
    try {
      const encoded = encodeURIComponent(selection.chat)
      const result = await request<Page<ChatMessage>>(`/api/v1/instances/${instance.id}/chats/${encoded}/messages?page=${page}&pageSize=50`, { signal })
      if (signal?.aborted) return
      const incoming = [...result.data].reverse()
      setMessages((current) => {
        if (refresh) {
          const merged = new Map(current.map((item) => [item.eventId, item]))
          for (const item of incoming) merged.set(item.eventId, item)
          return [...merged.values()].sort((left, right) => new Date(left.timestamp).getTime() - new Date(right.timestamp).getTime())
        }
        if (!append) return incoming
        const known = new Set(current.map((item) => item.eventId))
        return [...incoming.filter((item) => !known.has(item.eventId)), ...current]
      })
      if (!refresh) setMessagePage(result.page)
      setMessagePages(Math.max(result.totalPages, 1)); setError('')
      await request(`/api/v1/instances/${instance.id}/chats/${encoded}/read`, { method: 'POST', signal })
      if (!signal?.aborted) void loadList(true, signal)
    } catch (reason) {
      if (!signal?.aborted) setError(reason instanceof Error ? reason.message : 'Falha ao carregar mensagens.')
    } finally {
      if (!signal?.aborted) { setLoadingMessages(false); setLoadingOlder(false) }
    }
  }, [instance.id, loadList])

  useEffect(() => {
    if (!selected) return
    const controller = new AbortController()
    const timer = window.setInterval(() => void loadMessages(selected, 1, false, controller.signal, true), 3500)
    return () => { controller.abort(); window.clearInterval(timer) }
  }, [loadMessages, selected])

  useEffect(() => {
    if (!loadingMessages && messages.length) messageEnd.current?.scrollIntoView({ block: 'end' })
  }, [loadingMessages, messages.length, selected])

  function choose(selection: Selection) {
    setSelected(selection); setMessages([]); setMessagePage(1); setMessagePages(1); setError('')
    void loadMessages(selection)
  }

  async function send(event: FormEvent) {
    event.preventDefault()
    if (!selected || !draft.trim() || sending) return
    setSending(true); setError('')
    const text = draft.trim()
    try {
      const encoded = encodeURIComponent(selected.chat)
      await request(`/api/v1/instances/${instance.id}/chats/${encoded}/messages`, {
        method: 'POST', body: JSON.stringify({ message: text }),
      })
      setDraft('')
      await new Promise((resolve) => window.setTimeout(resolve, 250))
      await loadMessages(selected)
      await loadList(true)
    } catch (reason) {
      setError(reason instanceof Error ? reason.message : 'Não foi possível enviar a mensagem.')
    } finally { setSending(false) }
  }

  function composerKeyDown(event: KeyboardEvent<HTMLTextAreaElement>) {
    if (event.key === 'Enter' && !event.shiftKey) {
      event.preventDefault()
      event.currentTarget.form?.requestSubmit()
    }
  }

  const listItems = view === 'chats' ? chats?.data ?? [] : contacts?.data ?? []
  const total = view === 'chats' ? chats?.total ?? 0 : contacts?.total ?? 0
  const title = selected ? chatName(selected.chat, selected.name, selected.isGroup) : ''
  const connected = instance.status === 'connected'
  const writable = connected && canSend
  const renderedMessages = useMemo(() => messages.map((item) => ({ ...item, body: preview(item) })), [messages])

  return (
    <dialog ref={dialog} className={`inboxModal ${selected ? 'hasConversation' : ''}`} aria-labelledby="inbox-title"
      onCancel={(event) => { event.preventDefault(); if (!sending) onClose() }}>
      <header className="manageHeader inboxHeader">
        <div><p className="eyebrow">CENTRAL DE CONVERSAS</p><h2 id="inbox-title">Conversas · {instance.name}</h2>
          <span className={`badge badge-${instance.status}`}>{writable ? 'Online para responder' : connected ? 'Somente leitura · seu papel não permite enviar' : 'Somente leitura · instância desconectada'}</span></div>
        <button className="closeButton" type="button" aria-label="Fechar conversas" disabled={sending} onClick={onClose}>×</button>
      </header>

      <div className="inboxBody">
        <aside className="inboxSidebar">
          <div className="inboxTabs" role="tablist" aria-label="Conteúdo da caixa de entrada">
            <button type="button" role="tab" aria-selected={view === 'chats'} onClick={() => { setView('chats'); setSearch('') }}>Conversas</button>
            <button type="button" role="tab" aria-selected={view === 'contacts'} onClick={() => { setView('contacts'); setSearch('') }}>Contatos</button>
          </div>
          <label className="inboxSearch"><span aria-hidden="true">⌕</span><span className="visuallyHidden">Buscar {view === 'chats' ? 'conversas' : 'contatos'}</span>
            <input value={search} onChange={(event) => setSearch(event.target.value)} placeholder={view === 'chats' ? 'Buscar conversa' : 'Buscar nome ou número'} maxLength={100} /></label>
          {error && !selected && <p className="inboxSidebarError">{error}</p>}
          <div className="inboxListMeta"><span>{view === 'chats' ? 'Recentes' : 'Agenda do WhatsApp'}</span><small>{total}</small></div>
          <div className="inboxList">
            {loadingList && !listItems.length ? <div className="inboxListLoading"><span className="spinner" /></div> : listItems.length === 0 ?
              <div className="inboxListEmpty"><span>○</span><p>{search ? 'Nenhum resultado encontrado.' : view === 'chats' ? 'As novas mensagens aparecerão aqui.' : 'Nenhum contato sincronizado.'}</p></div> :
              view === 'chats' ? (listItems as ChatSummary[]).map((item) => {
                const name = chatName(item.chat, item.name, item.isGroup)
                return <button className={`inboxRow ${selected?.chat === item.chat ? 'active' : ''}`} type="button" key={item.chat}
                  onClick={() => choose({ chat: item.chat, name: item.name, isGroup: item.isGroup })}>
                  <span className="inboxAvatar">{initials(name)}</span><span className="inboxRowContent"><span><strong>{name}</strong><time>{shortTime(item.lastMessage.timestamp)}</time></span>
                    <span><small>{item.lastMessage.fromMe && 'Você: '}{preview(item.lastMessage)}</small>{item.unreadCount > 0 && <b>{item.unreadCount > 99 ? '99+' : item.unreadCount}</b>}</span></span>
                </button>
              }) : (listItems as Contact[]).map((item) => <button className={`inboxRow ${selected?.chat === item.jid ? 'active' : ''}`} type="button" key={item.jid}
                onClick={() => choose({ chat: item.jid, name: item.name, isGroup: false })}>
                <span className="inboxAvatar contact">{initials(item.name)}</span><span className="inboxRowContent"><span><strong>{item.name}</strong></span>
                  <span><small>{item.phone || item.jid}</small></span></span>
              </button>)}
          </div>
          {total > 100 && <p className="inboxLimit">Mostrando 100 itens. Use a busca para refinar.</p>}
        </aside>

        <section className="conversationPane" aria-label={selected ? `Conversa com ${title}` : 'Selecione uma conversa'}>
          {!selected ? <div className="conversationEmpty"><span className="conversationMark">W</span><h3>Selecione uma conversa</h3><p>Leia mensagens e responda sem sair do painel Wirely.</p></div> : <>
            <header className="conversationHeader"><button className="conversationBack" type="button" aria-label="Voltar para conversas" onClick={() => setSelected(null)}>‹</button>
              <span className="inboxAvatar">{initials(title)}</span><div><strong>{title}</strong><small>{selected.isGroup ? 'Grupo do WhatsApp' : selected.chat}</small></div></header>
            <div className="messageTimeline">
              {messagePage < messagePages && <button className="loadOlder" type="button" disabled={loadingOlder} onClick={() => void loadMessages(selected, messagePage + 1, true)}>{loadingOlder ? 'Carregando…' : 'Carregar mensagens anteriores'}</button>}
              {loadingMessages && !messages.length ? <div className="messageLoading"><span className="spinner" /></div> : messages.length === 0 ?
                <div className="messageEmpty"><p>Nenhuma mensagem recente com este contato.</p><small>Você pode iniciar a conversa abaixo.</small></div> :
                renderedMessages.map((item) => <article className={`messageBubble ${item.fromMe ? 'outgoing' : 'incoming'} event-${item.event.split('.')[1]}`} key={item.eventId}>
                  {!item.fromMe && selected.isGroup && <strong>{item.pushName || (item.sender ?? '').split('@')[0]}</strong>}
                  <p>{item.body}</p><span>{item.event === 'message.updated' && 'editada · '}{fullTime(item.timestamp)}</span>
                </article>)}
              <div ref={messageEnd} />
            </div>
            {error && <p className="inboxError">{error}</p>}
            <form className="conversationComposer" onSubmit={send}>
              <textarea aria-label="Mensagem da conversa" value={draft} onChange={(event) => setDraft(event.target.value)} onKeyDown={composerKeyDown}
                placeholder={writable ? 'Digite uma mensagem' : canSend ? 'Conecte a instância para responder' : 'Seu papel permite apenas leitura'} maxLength={4096} rows={1} disabled={!writable || sending} />
              <button className="primaryButton" type="submit" disabled={!writable || !draft.trim() || sending} aria-busy={sending}>{sending ? 'Enviando…' : 'Enviar'}</button>
              <small>Enter envia · Shift + Enter quebra a linha</small>
            </form>
          </>}
        </section>
      </div>
    </dialog>
  )
}
