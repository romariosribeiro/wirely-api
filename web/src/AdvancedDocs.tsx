import { useMemo, useState } from 'react'
import { useLanguage } from './i18n'

type Route = {
  method: 'GET' | 'POST' | 'PUT' | 'PATCH' | 'DELETE'
  path: string
  examplePath?: string
  pt: string
  en: string
  descriptionPt: string
  descriptionEn: string
  body?: string
  form?: string[]
  stream?: boolean
}

const group = '120363012345678901@g.us'
const routes: Route[] = [
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

  async function copy() {
    try {
      await navigator.clipboard.writeText(code)
    } catch {
      const area = document.createElement('textarea')
      area.value = code; area.style.position = 'fixed'; area.style.opacity = '0'
      document.body.appendChild(area); area.select(); document.execCommand('copy'); area.remove()
    }
    setCopied(true); window.setTimeout(() => setCopied(false), 1800)
  }

  return <section className="docsSection" aria-labelledby="docs-advanced-title">
    <div className="docsSectionTitle"><div>
      <h3 id="docs-advanced-title">{english ? 'Advanced API' : 'API avançada'}</h3>
      <p>{english ? 'Select an endpoint to inspect and copy a ready-to-use example.' : 'Selecione um endpoint para consultar e copiar um exemplo pronto.'}</p>
    </div><span className="docsQueueBadge">17 {english ? 'ROUTES' : 'ROTAS'}</span></div>
    <div className="docsEndpoints docsGroupEndpoints">
      {routes.map((item, index) => <button type="button" className={selected === index ? 'active' : ''} key={`${item.method}-${item.path}`} onClick={() => { setSelected(index); setCopied(false) }}>
        <span className={`method-${item.method.toLowerCase()}`}>{item.method}</span><code>{item.path}</code><strong>{english ? item.en : item.pt}</strong>
      </button>)}
    </div>
    <div className="docsEndpointNote"><strong>{english ? route.en : route.pt}</strong><span>{english ? route.descriptionEn : route.descriptionPt}</span></div>
    <div className="docsCodeHeader docsGroupCodeHeader"><span>{english ? 'cURL example' : 'Exemplo cURL'}</span>
      <button className="secondaryButton" type="button" onClick={() => void copy()}>{copied ? (english ? 'Copied ✓' : 'Copiado ✓') : (english ? 'Copy code' : 'Copiar código')}</button></div>
    <pre className="docsCode docsGroupCode"><code>{code}</code></pre>
    <p className="docsSecurity">{english ? 'Webhooks remain recommended for durable delivery; SSE is ideal for live dashboards.' : 'Webhooks continuam recomendados para entrega durável; SSE é ideal para painéis ao vivo.'}</p>
  </section>
}
