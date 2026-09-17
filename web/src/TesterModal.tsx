import { useEffect, useRef, useState, type FormEvent } from 'react'
import { request, type Instance, type SentMessage } from './api'

type MediaType = 'image' | 'video' | 'audio' | 'document' | 'sticker'
type MessageType = 'text' | MediaType | 'location' | 'contact' | 'poll' | 'reaction'

const types: { id: MessageType; label: string; accept?: string }[] = [
  { id: 'text', label: 'Texto' },
  { id: 'image', label: 'Imagem', accept: 'image/*' },
  { id: 'video', label: 'Vídeo', accept: 'video/*' },
  { id: 'audio', label: 'Áudio', accept: 'audio/*,.ogg' },
  { id: 'document', label: 'Documento', accept: '.pdf,.doc,.docx,.xls,.xlsx,.csv,.txt,.zip,application/*,text/*' },
  { id: 'sticker', label: 'Figurinha', accept: '.webp,image/webp' },
  { id: 'location', label: 'Localização' },
  { id: 'contact', label: 'Contato' },
  { id: 'poll', label: 'Enquete' },
  { id: 'reaction', label: 'Reação' },
]

function isMediaType(type: MessageType): type is MediaType {
  return ['image', 'video', 'audio', 'document', 'sticker'].includes(type)
}

export function TesterModal({ instance, onClose, onManage }: {
  instance: Instance
  onClose: () => void
  onManage: () => void
}) {
  const dialog = useRef<HTMLDialogElement>(null)
  const fileInput = useRef<HTMLInputElement>(null)
  const [type, setType] = useState<MessageType>('text')
  const [recipient, setRecipient] = useState('')
  const [message, setMessage] = useState('')
  const [caption, setCaption] = useState('')
  const [file, setFile] = useState<File | null>(null)
  const [voice, setVoice] = useState(false)
  const [latitude, setLatitude] = useState('-23.5505')
  const [longitude, setLongitude] = useState('-46.6333')
  const [placeName, setPlaceName] = useState('')
  const [address, setAddress] = useState('')
  const [contactName, setContactName] = useState('')
  const [contactOrganization, setContactOrganization] = useState('')
  const [contactPhone, setContactPhone] = useState('')
  const [pollQuestion, setPollQuestion] = useState('')
  const [pollOptions, setPollOptions] = useState('Sim\nNão')
  const [maxAnswer, setMaxAnswer] = useState('1')
  const [messageID, setMessageID] = useState('')
  const [reaction, setReaction] = useState('👍')
  const [fromMe, setFromMe] = useState(false)
  const [participant, setParticipant] = useState('')
  const [token, setToken] = useState('')
  const [loadingToken, setLoadingToken] = useState(true)
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')
  const [result, setResult] = useState<SentMessage | null>(null)
  const [presence, setPresence] = useState<'composing' | 'recording'>('composing')
  const [delay, setDelay] = useState('2000')

  useEffect(() => {
    const element = dialog.current!
    const previousFocus = document.activeElement as HTMLElement | null
    element.showModal()
    return () => { element.close(); previousFocus?.focus() }
  }, [])

  useEffect(() => {
    const controller = new AbortController()
    request<{ token: string }>(`/api/v1/instances/${instance.id}/token`, { signal: controller.signal })
      .then((value) => setToken(value.token))
      .catch((reason) => setError(reason instanceof Error ? reason.message : 'Falha ao carregar token.'))
      .finally(() => { if (!controller.signal.aborted) setLoadingToken(false) })
    return () => controller.abort()
  }, [instance.id])

  function changeType(next: MessageType) {
    setType(next); setFile(null); setResult(null); setError('')
    setPresence(next === 'audio' ? 'recording' : 'composing')
    if (fileInput.current) fileInput.current.value = ''
  }

  async function submit(event: FormEvent) {
    event.preventDefault()
    if (!token) { setError('Esta instância ainda não possui um token visível. Gere um token em Gerenciar.'); return }
    setBusy(true); setError(''); setResult(null)
    try {
      let response: SentMessage
      if (isMediaType(type)) {
        if (!file) throw new Error('Selecione um arquivo para enviar.')
        const data = new FormData()
        data.append('recipient', recipient)
        data.append('type', type)
        data.append('options', JSON.stringify({ presence, delay: Number(delay) }))
        data.append('file', file)
        if (caption && type !== 'audio' && type !== 'sticker') data.append('caption', caption)
        if (type === 'audio') data.append('voice', String(voice))
        response = await request<SentMessage>('/api/send/media', {
          method: 'POST', headers: { Authorization: `Bearer ${token}` }, body: data,
        })
      } else {
        let payload: Record<string, unknown> = { recipient, options: { presence, delay: Number(delay) } }
        if (type === 'text') payload = { ...payload, message }
        if (type === 'location') payload = { ...payload, latitude: Number(latitude), longitude: Number(longitude), name: placeName, address }
        if (type === 'contact') payload = { ...payload, fullName: contactName, organization: contactOrganization, phone: contactPhone }
        if (type === 'poll') payload = {
          ...payload, question: pollQuestion,
          choices: pollOptions.split('\n').map((option) => option.trim()).filter(Boolean),
          maxAnswer: Number(maxAnswer),
        }
        if (type === 'reaction') payload = {
          ...payload, messageId: messageID, reaction, fromMe,
          ...(participant.trim() ? { participant: participant.trim() } : {}),
        }
        response = await request<SentMessage>(`/api/send/${type}`, {
          method: 'POST', headers: { Authorization: `Bearer ${token}` }, body: JSON.stringify(payload),
        })
      }
      setResult(response)
    } catch (reason) {
      setError(reason instanceof Error ? reason.message : 'Não foi possível enviar a mensagem.')
    } finally { setBusy(false) }
  }

  const selected = types.find((item) => item.id === type)!
  const endpointPath = isMediaType(type) ? '/api/send/media' : `/api/send/${type}`
  return (
    <dialog ref={dialog} className="testerModal" aria-labelledby="tester-title"
      onCancel={(event) => { event.preventDefault(); if (!busy) onClose() }}>
      <header className="manageHeader">
        <div><p className="eyebrow">PLAYGROUND DA API</p><h2 id="tester-title">Testar envio · {instance.name}</h2>
          <span className="badge badge-connected">Conectada</span></div>
        <button className="closeButton" type="button" aria-label="Fechar testador" disabled={busy} onClick={onClose}>×</button>
      </header>
      <form className="testerBody" onSubmit={submit}>
        <div className="testerTabs" role="tablist" aria-label="Tipo da mensagem">
          {types.map((item) => <button key={item.id} type="button" role="tab" aria-selected={type === item.id}
            onClick={() => changeType(item.id)}>{item.label}</button>)}
        </div>

        {!loadingToken && !token && <div className="testerTokenWarning">
          <div><strong>Token não disponível</strong><p>Gere um novo token uma única vez para usar o testador.</p></div>
          <button className="secondaryButton" type="button" onClick={onManage}>Abrir Gerenciar</button>
        </div>}

        <label className="testerLabel" htmlFor="tester-recipient">Número com código do país ou JID do grupo</label>
        <input id="tester-recipient" value={recipient} onChange={(event) => setRecipient(event.target.value)}
          placeholder="Ex.: 5511999999999" inputMode="numeric" autoComplete="tel" required disabled={busy} />
        <section className="testerSendOptions" aria-labelledby="tester-options-title">
          <div className="testerOptionsHeading"><div><strong id="tester-options-title">Simular presença</strong>
            <span>O status aparece no WhatsApp antes da mensagem.</span></div><code>options</code></div>
          <div className="testerOptionsGrid">
            <label htmlFor="tester-presence"><span>Presença</span>
              <select id="tester-presence" value={presence}
                onChange={(event) => setPresence(event.target.value as 'composing' | 'recording')} disabled={busy}>
                <option value="composing">Digitando…</option>
                <option value="recording">Gravando áudio…</option>
              </select>
            </label>
            <label htmlFor="tester-delay"><span>Duração</span>
              <div className="testerDelayInput"><input id="tester-delay" type="number" min="0" max="60000" step="100"
                value={delay} onChange={(event) => setDelay(event.target.value)} required disabled={busy} /><small>ms</small></div>
            </label>
          </div>
          <small className="testerOptionsHelp">O Wirely envia a presença, aguarda o tempo escolhido, envia a mensagem e finaliza com <code>paused</code>.</small>
        </section>


        {type === 'text' && <>
          <label className="testerLabel" htmlFor="tester-message">Mensagem</label>
          <textarea id="tester-message" value={message} onChange={(event) => setMessage(event.target.value)}
            placeholder="Digite a mensagem de teste" maxLength={4096} required disabled={busy} />
          <small className="characterCount">{message.length}/4096</small>
        </>}

        {isMediaType(type) && <>
          <label className="testerLabel" htmlFor="tester-file">Arquivo {selected.label.toLowerCase()}</label>
          <label className={`filePicker ${file ? 'hasFile' : ''}`} htmlFor="tester-file">
            <span className="filePickerIcon">↑</span>
            <span><strong>{file?.name ?? 'Selecionar arquivo'}</strong>
              <small>{file ? `${(file.size / 1024 / 1024).toFixed(2)} MB` : 'Tamanho máximo de 32 MB'}</small></span>
          </label>
          <input ref={fileInput} className="visuallyHidden" id="tester-file" type="file" accept={selected.accept}
            required onChange={(event) => setFile(event.target.files?.[0] ?? null)} disabled={busy} />
          {type !== 'audio' && type !== 'sticker' && <>
            <label className="testerLabel" htmlFor="tester-caption">Legenda <span>opcional</span></label>
            <textarea id="tester-caption" value={caption} onChange={(event) => setCaption(event.target.value)}
              placeholder="Adicione uma legenda" maxLength={1024} disabled={busy} />
          </>}
          {type === 'audio' && <label className="testerCheckbox"><input type="checkbox" checked={voice}
            onChange={(event) => setVoice(event.target.checked)} disabled={busy} />Enviar como mensagem de voz</label>}
        </>}

        {type === 'location' && <>
          <label className="testerLabel" htmlFor="tester-latitude">Latitude</label>
          <input id="tester-latitude" type="number" step="any" min="-90" max="90" value={latitude}
            onChange={(event) => setLatitude(event.target.value)} required disabled={busy} />
          <label className="testerLabel" htmlFor="tester-longitude">Longitude</label>
          <input id="tester-longitude" type="number" step="any" min="-180" max="180" value={longitude}
            onChange={(event) => setLongitude(event.target.value)} required disabled={busy} />
          <label className="testerLabel" htmlFor="tester-place">Nome do local <span>opcional</span></label>
          <input id="tester-place" value={placeName} onChange={(event) => setPlaceName(event.target.value)}
            maxLength={200} placeholder="Ex.: Praça da Sé" disabled={busy} />
          <label className="testerLabel" htmlFor="tester-address">Endereço <span>opcional</span></label>
          <input id="tester-address" value={address} onChange={(event) => setAddress(event.target.value)}
            maxLength={500} placeholder="Ex.: São Paulo - SP" disabled={busy} />
        </>}

        {type === 'contact' && <>
          <label className="testerLabel" htmlFor="tester-contact-name">Nome do contato</label>
          <input id="tester-contact-name" value={contactName} onChange={(event) => setContactName(event.target.value)}
            maxLength={200} required disabled={busy} />
          <label className="testerLabel" htmlFor="tester-contact-phone">Telefone do contato</label>
          <input id="tester-contact-phone" value={contactPhone} onChange={(event) => setContactPhone(event.target.value)}
            placeholder="5511888888888" inputMode="numeric" required disabled={busy} />
          <label className="testerLabel" htmlFor="tester-contact-org">Organização <span>opcional</span></label>
          <input id="tester-contact-org" value={contactOrganization} onChange={(event) => setContactOrganization(event.target.value)}
            maxLength={200} disabled={busy} />
        </>}

        {type === 'poll' && <>
          <label className="testerLabel" htmlFor="tester-poll-question">Pergunta</label>
          <input id="tester-poll-question" value={pollQuestion} onChange={(event) => setPollQuestion(event.target.value)}
            maxLength={255} required disabled={busy} />
          <label className="testerLabel" htmlFor="tester-poll-options">Opções <span>uma por linha</span></label>
          <textarea id="tester-poll-options" value={pollOptions} onChange={(event) => setPollOptions(event.target.value)}
            placeholder={'Opção A\nOpção B'} required disabled={busy} />
          <label className="testerLabel" htmlFor="tester-max-answer">Máximo de respostas</label>
          <input id="tester-max-answer" type="number" min="1" max="12" value={maxAnswer}
            onChange={(event) => setMaxAnswer(event.target.value)} required disabled={busy} />
        </>}

        {type === 'reaction' && <>
          <label className="testerLabel" htmlFor="tester-message-id">ID da mensagem</label>
          <input id="tester-message-id" value={messageID} onChange={(event) => setMessageID(event.target.value)}
            maxLength={200} required disabled={busy} />
          <label className="testerLabel" htmlFor="tester-reaction">Reação <span>vazio remove a atual</span></label>
          <input id="tester-reaction" value={reaction} onChange={(event) => setReaction(event.target.value)}
            maxLength={16} placeholder="👍" disabled={busy} />
          <label className="testerCheckbox"><input type="checkbox" checked={fromMe}
            onChange={(event) => setFromMe(event.target.checked)} disabled={busy} />A mensagem alvo foi enviada por esta instância</label>
          {!fromMe && <><label className="testerLabel" htmlFor="tester-participant">Autor no grupo <span>somente para grupos</span></label>
            <input id="tester-participant" value={participant} onChange={(event) => setParticipant(event.target.value)}
              placeholder="Telefone ou JID do participante" disabled={busy} /></>}
        </>}

        {error && <p className="formError testerFeedback" role="alert">{error}</p>}
        {result && <div className="testerResult" role="status">
          <span className="testerResultIcon">✓</span><div><strong>Mensagem enviada</strong>
            <dl><div><dt>ID</dt><dd>{result.id}</dd></div><div><dt>Destino</dt><dd>{result.recipient}</dd></div>
              <div><dt>Tipo</dt><dd>{result.type}</dd></div><div><dt>Horário</dt><dd>{new Date(result.timestamp).toLocaleString('pt-BR')}</dd></div></dl>
          </div>
        </div>}
        <div className="testerEndpoint"><span>POST</span><code>{endpointPath}</code></div>
        <footer className="testerActions">
          <button className="secondaryButton" type="button" disabled={busy} onClick={onClose}>Fechar</button>
          <button className="primaryButton" type="submit" disabled={busy || loadingToken || !token} aria-busy={busy}>
            {busy ? 'Enviando…' : 'Enviar teste'}
          </button>
        </footer>
      </form>
    </dialog>
  )
}
