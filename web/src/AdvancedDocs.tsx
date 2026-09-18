import { useLanguage } from './i18n'

const routes = [
  ['POST', '/api/send/location/live', 'Localização ao vivo (experimental)', 'Live location (experimental)'],
  ['POST', '/api/messages/forward', 'Encaminhar texto ou mídia', 'Forward text or media'],
  ['PUT', '/api/chats/disappearing', 'Mensagens temporárias', 'Disappearing messages'],
  ['POST', '/api/presence', 'Presença global', 'Global presence'],
  ['POST', '/api/presence/subscribe', 'Assinar presença de contato', 'Subscribe to contact presence'],
  ['GET', '/api/presence/{phone}', 'Consultar presença', 'Get presence'],
  ['POST', '/api/status/text', 'Publicar Status de texto', 'Publish text Status'],
  ['POST', '/api/status/media', 'Publicar Status com imagem ou vídeo', 'Publish image or video Status'],
  ['GET', '/api/events', 'Eventos em tempo real por SSE', 'Real-time events over SSE'],
  ['PATCH', '/api/groups/{groupJID}/description', 'Alterar descrição', 'Update description'],
  ['PUT', '/api/groups/{groupJID}/photo', 'Definir foto', 'Set photo'],
  ['DELETE', '/api/groups/{groupJID}/photo', 'Remover foto', 'Remove photo'],
  ['POST', '/api/groups/{groupJID}/leave', 'Sair do grupo', 'Leave group'],
  ['PATCH', '/api/groups/{groupJID}/permissions', 'Permissões de envio e edição', 'Send and edit permissions'],
  ['PUT', '/api/groups/{groupJID}/join-approval', 'Aprovação de entrada', 'Join approval'],
  ['GET', '/api/groups/{groupJID}/join-requests', 'Listar solicitações', 'List join requests'],
  ['POST', '/api/groups/{groupJID}/join-requests', 'Aprovar ou rejeitar', 'Approve or reject'],
] as const

export function AdvancedDocs() {
  const { language } = useLanguage()
  const english = language === 'en'
  return <section className="docsSection" aria-labelledby="docs-advanced-title">
    <div className="docsSectionTitle"><div>
      <h3 id="docs-advanced-title">{english ? 'Advanced API' : 'API avançada'}</h3>
      <p>{english ? 'Status, presence, live events, advanced messages, and full group administration.' : 'Status, presença, eventos ao vivo, mensagens avançadas e administração completa de grupos.'}</p>
    </div><span className="docsQueueBadge">17 {english ? 'ROUTES' : 'ROTAS'}</span></div>
    <div className="docsEndpoints docsGroupEndpoints">
      {routes.map(([method, path, pt, en]) => <div className="docsStaticEndpoint" key={`${method}-${path}`}>
        <span className={`method-${method.toLowerCase()}`}>{method}</span><code>{path}</code><strong>{english ? en : pt}</strong>
      </div>)}
    </div>
    <div className="docsEndpointNote"><strong>SSE</strong><span>{english ? 'Use webhooks for durable server-to-server delivery and SSE for live dashboards. The stream supports event filters and automatic reconnection.' : 'Use webhooks para entrega durável entre servidores e SSE para painéis ao vivo. O fluxo aceita filtros de eventos e reconexão automática.'}</span></div>
    <pre className="docsCode docsGroupCode"><code>{`curl -N '${window.location.origin}/api/events?events=message.received,presence.updated' \\\n  -H 'Authorization: Bearer wly_SEU_TOKEN'`}</code></pre>
    <p className="docsSecurity">{english ? 'Download ready-to-import Postman and Bruno collections from the repository collections folder. The PHP SDK is under sdk/php.' : 'Baixe as coleções Postman e Bruno prontas para importar na pasta collections do repositório. O SDK PHP está em sdk/php.'}</p>
  </section>
}
