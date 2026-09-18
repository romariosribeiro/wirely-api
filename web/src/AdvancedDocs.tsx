import { type MouseEvent, useMemo, useState } from 'react'
import { useLanguage } from './i18n'
import { EndpointFields } from './EndpointFields'

type Route = {
  method: 'GET' | 'POST' | 'PUT' | 'PATCH' | 'DELETE'
  path: string
  examplePath?: string
  pt: string
  en: string
  descriptionPt: string
  descriptionEn: string
  requiredPt?: string[]
  requiredEn?: string[]
  optionalPt?: string[]
  optionalEn?: string[]
  body?: string
  form?: string[]
  stream?: boolean
}

const group = '120363012345678901@g.us'
const routes: Route[] = [
  { method: 'POST', path: '/api/send/text', pt: 'Texto com recursos opcionais', en: 'Text with optional features', descriptionPt: 'É o mesmo endpoint de texto já existente. Os recursos avançados são campos opcionais no body.', descriptionEn: 'This is the same existing text endpoint. Advanced features are optional body fields.', requiredPt: ['recipient — número do destinatário sem +', 'message — texto que será enviado'], requiredEn: ['recipient — destination number without +', 'message — text to send'], optionalPt: ['replyTo — messageId e participant (participant somente em grupos)', 'mentions — números que serão mencionados', 'linkPreview — gera a prévia de links', 'forwarded — exibe a indicação de encaminhada', 'options — presence e delay em milissegundos'], optionalEn: ['replyTo — messageId and participant (participant only for groups)', 'mentions — numbers to mention', 'linkPreview — generates a link preview', 'forwarded — displays the forwarded indication', 'options — presence and delay in milliseconds'], body: '{"recipient":"5511999999999","message":"Respondendo sua mensagem","replyTo":{"messageId":"3EB0ABC123","participant":""},"mentions":["5511888888888"],"linkPreview":true,"options":{"presence":"composing","delay":2000}}' },
  { method: 'POST', path: '/api/send/media', pt: 'Mídia com recursos opcionais', en: 'Media with optional features', descriptionPt: 'É o mesmo endpoint multipart de mídia. Os recursos de contexto ficam em messageOptions.', descriptionEn: 'This is the same multipart media endpoint. Context features are passed in messageOptions.', requiredPt: ['recipient — número do destinatário sem +', 'type — image, video, audio, document ou sticker', 'file — arquivo enviado como multipart'], requiredEn: ['recipient — destination number without +', 'type — image, video, audio, document, or sticker', 'file — multipart file upload'], optionalPt: ['messageOptions — JSON com replyTo, mentions e forwarded', 'viewOnce — mídia de visualização única', 'caption — legenda da mídia', 'options — presence e delay em milissegundos'], optionalEn: ['messageOptions — JSON with replyTo, mentions, and forwarded', 'viewOnce — view-once media', 'caption — media caption', 'options — presence and delay in milliseconds'], form: ['recipient=5511999999999', 'type=image', 'messageOptions={"replyTo":{"messageId":"3EB0ABC123"},"mentions":["5511888888888"],"forwarded":false}', 'viewOnce=true', 'file=@./foto.jpg;type=image/jpeg'] },
  { method: 'POST', path: '/api/send/location', pt: 'Localização com recursos opcionais', en: 'Location with optional features', descriptionPt: 'Reutiliza o endpoint de localização e adiciona contexto sem alterar o uso básico.', descriptionEn: 'Reuses the location endpoint and adds context without changing basic usage.', requiredPt: ['recipient — número do destinatário sem +', 'latitude — latitude decimal', 'longitude — longitude decimal'], requiredEn: ['recipient — destination number without +', 'latitude — decimal latitude', 'longitude — decimal longitude'], optionalPt: ['replyTo, mentions e forwarded — contexto da mensagem', 'name e address — identificação do local', 'options — presence e delay em milissegundos'], optionalEn: ['replyTo, mentions, and forwarded — message context', 'name and address — location identification', 'options — presence and delay in milliseconds'], body: '{"recipient":"5511999999999","latitude":-23.5505,"longitude":-46.6333,"replyTo":{"messageId":"3EB0ABC123"},"mentions":["5511888888888"],"options":{"presence":"composing","delay":2000}}' },
  { method: 'POST', path: '/api/send/contact', pt: 'Contato com recursos opcionais', en: 'Contact with optional features', descriptionPt: 'Reutiliza o endpoint de contato e aceita os mesmos recursos de contexto.', descriptionEn: 'Reuses the contact endpoint and accepts the same context features.', requiredPt: ['recipient — número do destinatário sem +', 'fullName — nome do contato', 'phone — telefone do contato sem +'], requiredEn: ['recipient — destination number without +', 'fullName — contact name', 'phone — contact phone without +'], optionalPt: ['replyTo, mentions e forwarded — contexto da mensagem', 'organization e email — dados adicionais do contato', 'options — presence e delay em milissegundos'], optionalEn: ['replyTo, mentions, and forwarded — message context', 'organization and email — additional contact data', 'options — presence and delay in milliseconds'], body: '{"recipient":"5511999999999","fullName":"Maria Silva","phone":"5511888888888","replyTo":{"messageId":"3EB0ABC123"},"mentions":["5511777777777"],"forwarded":false,"options":{"presence":"composing","delay":2000}}' },
  { method: 'POST', path: '/api/send/location/live', pt: 'Localização ao vivo (experimental)', en: 'Live location (experimental)', descriptionPt: 'Envia uma atualização com coordenadas, precisão, movimento e sequência.', descriptionEn: 'Sends an update with coordinates, accuracy, motion, and sequence.', body: '{"recipient":"5511999999999","latitude":-23.5505,"longitude":-46.6333,"accuracy":10,"sequence":1}' },
  { method: 'POST', path: '/api/messages/forward', pt: 'Encaminhar texto ou mídia', en: 'Forward text or media', descriptionPt: 'Encaminha uma mensagem existente no histórico e cache locais.', descriptionEn: 'Forwards a message available in local history and cache.', body: '{"chat":"5511999999999","messageId":"3EB0ABC123","recipient":"5511888888888"}' },
  { method: 'PUT', path: '/api/chats/disappearing', pt: 'Mensagens temporárias', en: 'Disappearing messages', descriptionPt: 'Configura o período oficial: off, 24h, 7d ou 90d.', descriptionEn: 'Sets an official period: off, 24h, 7d, or 90d.', body: '{"chat":"5511999999999","duration":"7d"}' },
  { method: 'POST', path: '/api/presence', pt: 'Presença global', en: 'Global presence', descriptionPt: 'Define a instância como disponível ou indisponível.', descriptionEn: 'Sets the instance as available or unavailable.', body: '{"presence":"available"}' },
  { method: 'POST', path: '/api/presence/subscribe', pt: 'Assinar presença de contato', en: 'Subscribe to contact presence', descriptionPt: 'Passa a receber presence.updated para o contato.', descriptionEn: 'Starts receiving presence.updated for the contact.', body: '{"phone":"5511999999999"}' },
  { method: 'GET', path: '/api/presence/{phone}', examplePath: '/api/presence/5511999999999', pt: 'Consultar presença', en: 'Get presence', descriptionPt: 'Retorna o último estado conhecido do contato.', descriptionEn: 'Returns the contact’s last known state.' },
  { method: 'POST', path: '/api/status/text', pt: 'Publicar Status de texto', en: 'Publish text Status', descriptionPt: 'Publica texto com cores ARGB e fonte opcionais.', descriptionEn: 'Publishes text with optional ARGB colors and font.', body: '{"text":"Novidade no Wirely","background":4278190335,"textColor":4294967295,"font":0}' },
  { method: 'POST', path: '/api/status/media', pt: 'Publicar Status com mídia', en: 'Publish media Status', descriptionPt: 'Publica uma imagem ou um vídeo no Status.', descriptionEn: 'Publishes an image or video to Status.', form: ['type=image', 'caption=Novidade no Wirely', 'file=@./status.jpg;type=image/jpeg'] },
  { method: 'GET', path: '/api/events', examplePath: '/api/events?events=message.received,presence.updated', pt: 'Eventos em tempo real por SSE', en: 'Real-time events over SSE', descriptionPt: 'Mantém um fluxo autenticado com filtro, heartbeat e reconexão.', descriptionEn: 'Keeps an authenticated stream with filtering, heartbeat, and reconnection.', stream: true },
  { method: 'PATCH', path: '/api/groups/{groupJID}/description', examplePath: `/api/groups/${group}/description`, pt: 'Alterar descrição', en: 'Update description', descriptionPt: 'Define ou limpa a descrição do grupo.', descriptionEn: 'Sets or clears the group description.', body: '{"description":"Grupo oficial Wirely"}' },
  { method: 'PUT', path: '/api/groups/{groupJID}/photo', examplePath: `/api/groups/${group}/photo`, pt: 'Definir foto', en: 'Set photo', descriptionPt: 'Envia uma foto JPEG de até 5 MB.', descriptionEn: 'Uploads a JPEG photo up to 5 MB.', form: ['file=@./group.jpg;type=image/jpeg'] },
  { method: 'DELETE', path: '/api/groups/{groupJID}/photo', examplePath: `/api/groups/${group}/photo`, pt: 'Remover foto', en: 'Remove photo', descriptionPt: 'Remove a foto atual do grupo.', descriptionEn: 'Removes the current group photo.' },
  { method: 'POST', path: '/api/groups/{groupJID}/leave', examplePath: `/api/groups/${group}/leave`, pt: 'Sair do grupo', en: 'Leave group', descriptionPt: 'Faz a instância conectada sair do grupo.', descriptionEn: 'Makes the connected instance leave the group.' },
  { method: 'PATCH', path: '/api/groups/{groupJID}/permissions', examplePath: `/api/groups/${group}/permissions`, pt: 'Permissões de envio e edição', en: 'Send and edit permissions', descriptionPt: 'Restringe mensagens e edição das informações aos administradores.', descriptionEn: 'Restricts messages and group info editing to administrators.', body: '{"announce":true,"locked":true}' },
  { method: 'PUT', path: '/api/groups/{groupJID}/join-approval', examplePath: `/api/groups/${group}/join-approval`, pt: 'Aprovação de entrada', en: 'Join approval', descriptionPt: 'Ativa ou desativa aprovação de novos membros.', descriptionEn: 'Enables or disables approval of new members.', body: '{"enabled":true}' },
  { method: 'GET', path: '/api/groups/{groupJID}/join-requests', examplePath: `/api/groups/${group}/join-requests`, pt: 'Listar solicitações', en: 'List join requests', descriptionPt: 'Lista os contatos aguardando aprovação.', descriptionEn: 'Lists contacts waiting for approval.' },
  { method: 'POST', path: '/api/groups/{groupJID}/join-requests', examplePath: `/api/groups/${group}/join-requests`, pt: 'Aprovar ou rejeitar', en: 'Approve or reject', descriptionPt: 'Processa solicitações de entrada em lote.', descriptionEn: 'Processes join requests in a batch.', body: '{"action":"approve","participants":["5511999999999"]}' },
]

function curl(route: Route, token: string) {
  const continuation = ' \\\n  '
  const headers = [`-H 'Authorization: Bearer ${token || 'wly_SEU_TOKEN'}'`]
  if (route.body) headers.push("-H 'Content-Type: application/json'", `-d '${route.body}'`)
  if (route.form) headers.push(...route.form.map((field) => `-F '${field}'`))
  const stream = route.stream ? '-N ' : ''
  return `curl ${stream}-X ${route.method} '${window.location.origin}${route.examplePath ?? route.path}'${continuation}${headers.join(continuation)}`
}

export function AdvancedDocs({ token = '' }: { token?: string }) {
  const { language } = useLanguage()
  const english = language === 'en'
  const [selected, setSelected] = useState(0)
  const [copied, setCopied] = useState(false)
  const route = routes[selected]
  const code = useMemo(() => curl(route, token), [route, token])

  async function copy(event: MouseEvent<HTMLButtonElement>) {
    const container = event.currentTarget.closest('dialog') ?? document.body
    let done = false
    if (navigator.clipboard?.writeText) {
      try { await navigator.clipboard.writeText(code); done = true } catch { /* HTTP may deny Clipboard API. */ }
    }
    if (!done) {
      const previousFocus = document.activeElement as HTMLElement | null
      const area = document.createElement('textarea')
      area.value = code
      area.setAttribute('readonly', '')
      area.style.position = 'fixed'; area.style.inset = '0 auto auto 0'
      area.style.width = '1px'; area.style.height = '1px'; area.style.opacity = '0'
      container.appendChild(area)
      area.focus(); area.select(); area.setSelectionRange(0, area.value.length)
      done = document.execCommand('copy')
      area.remove()
      previousFocus?.focus()
    }
    if (!done) return
    setCopied(true); window.setTimeout(() => setCopied(false), 1800)
  }

  return <section className="docsSection" aria-labelledby="docs-advanced-title">
    <div className="docsSectionTitle"><div>
      <h3 id="docs-advanced-title">{english ? 'Advanced API' : 'API avançada'}</h3>
      <p>{english ? 'Select an endpoint to inspect and copy a ready-to-use example.' : 'Selecione um endpoint para consultar e copiar um exemplo pronto.'}</p>
    </div><span className="docsQueueBadge">{routes.length} {english ? 'ROUTES' : 'ROTAS'}</span></div>
    <div className="docsEndpoints docsGroupEndpoints">
      {routes.map((item, index) => <button type="button" className={selected === index ? 'active' : ''} key={`${item.method}-${item.path}`} onClick={() => { setSelected(index); setCopied(false) }}>
        <span className={`method-${item.method.toLowerCase()}`}>{item.method}</span><code>{item.path}</code><strong>{english ? item.en : item.pt}</strong>
      </button>)}
    </div>
    <div className="docsEndpointNote"><strong>{english ? route.en : route.pt}</strong><span>{english ? route.descriptionEn : route.descriptionPt}</span></div>
    {(route.requiredPt || route.optionalPt) && <div className="docsFieldGuide">
      <div><strong>{english ? 'Required fields' : 'Campos obrigatórios'}</strong>
        <ul>{(english ? route.requiredEn : route.requiredPt)?.map((field) => <li key={field}>{field}</li>)}</ul>
      </div>
      <div><strong>{english ? 'Optional fields' : 'Campos opcionais'}</strong>
        <ul>{(english ? route.optionalEn : route.optionalPt)?.map((field) => <li key={field}>{field}</li>)}</ul>
      </div>
    </div>}
    <EndpointFields path={route.path} method={route.method} hidden={Boolean(route.requiredPt || route.optionalPt)} />
    <div className="docsCodeHeader docsGroupCodeHeader"><span>{english ? 'cURL example' : 'Exemplo cURL'}</span>
      <button className="secondaryButton" type="button" onClick={(event) => void copy(event)}>{copied ? (english ? 'Copied ✓' : 'Copiado ✓') : (english ? 'Copy code' : 'Copiar código')}</button></div>
    <pre className="docsCode docsGroupCode"><code>{code}</code></pre>
    <p className="docsSecurity">{english ? 'Webhooks remain recommended for durable delivery; SSE is ideal for live dashboards.' : 'Webhooks continuam recomendados para entrega durável; SSE é ideal para painéis ao vivo.'}</p>
  </section>
}
