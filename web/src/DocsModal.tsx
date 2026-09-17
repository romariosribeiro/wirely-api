import { useEffect, useMemo, useRef, useState } from 'react'
import { request, type Instance } from './api'

type MediaEndpoint = 'image' | 'video' | 'audio' | 'document' | 'sticker'
type JSONEndpoint = 'text' | 'location' | 'contact' | 'poll' | 'reaction'
type Endpoint = JSONEndpoint | MediaEndpoint
type GroupEndpoint = 'list' | 'create' | 'join' | 'details' | 'rename' | 'participants' | 'invite' | 'rotate-invite'
type ProfileEndpoint = 'get-profile' | 'update-profile' | 'set-photo' | 'delete-photo' | 'get-privacy' | 'update-privacy'
type InstanceEndpoint = 'login' | 'all' | 'create' | 'settings' | 'delete' | 'details' | 'connect' | 'disconnect' | 'logout' | 'pair' | 'proxy' | 'qr' | 'status' | 'check'
type MessageActionEndpoint = 'delete-message' | 'edit-message' | 'mark-read' | 'message-status' | 'archive-chat' | 'mute-chat' | 'pin-chat' | 'unpin-chat'
type OrganizationEndpoint = 'create-newsletter' | 'get-newsletter' | 'invite-newsletter' | 'list-newsletters' | 'newsletter-messages' | 'subscribe-newsletter' | 'add-chat-label' | 'edit-label' | 'add-message-label' | 'remove-chat-label' | 'remove-message-label' | 'add-community-groups' | 'create-community' | 'remove-community-groups'
type Language = 'curl' | 'laravel' | 'node' | 'python'

type InstanceEndpointInfo = {
  id: InstanceEndpoint
  method: 'GET' | 'POST' | 'PUT' | 'DELETE'
  path: string
  title: string
  description: string
  admin?: boolean
  body?: string
}

const instanceEndpoints: InstanceEndpointInfo[] = [
  { id: 'login', method: 'POST', path: '/api/auth/login', title: 'Login administrativo', description: 'Autentica o administrador e salva a sessão usada nas rotas globais.', body: '{"username":"admin","password":"SUA_SENHA"}' },
  { id: 'all', method: 'GET', path: '/api/instances', title: 'Listar instâncias', description: 'Lista todas as instâncias sem expor seus tokens.', admin: true },
  { id: 'create', method: 'POST', path: '/api/instances', title: 'Criar instância', description: 'Cria a instância, define seu comportamento e retorna o primeiro token.', admin: true, body: '{"name":"Atendimento","alwaysOnline":false,"rejectCall":false,"msgRejectCall":"","readMessages":false,"ignoreGroups":false,"ignoreStatus":false}' },
  { id: 'settings', method: 'PUT', path: '/api/instances/{id}/settings', title: 'Configurar comportamento', description: 'Liga ou desliga presença contínua, rejeição de chamadas, leitura automática e filtros.', admin: true, body: '{"alwaysOnline":false,"rejectCall":false,"msgRejectCall":"","readMessages":false,"ignoreGroups":false,"ignoreStatus":false}' },
  { id: 'delete', method: 'DELETE', path: '/api/instances/{id}', title: 'Excluir instância', description: 'Exclui definitivamente sessão, filas, histórico e mídias.', admin: true },
  { id: 'details', method: 'GET', path: '/api/instance', title: 'Consultar instância', description: 'Retorna apenas a instância identificada pelo Bearer.' },
  { id: 'connect', method: 'POST', path: '/api/instance/connect', title: 'Conectar', description: 'Configura o webhook e conecta. Sem phone, aguarda até 30 segundos e retorna o QR em Base64; com phone, retorna o pairingCode.', body: '{"subscribe":["MESSAGE","SEND_MESSAGE","READ_RECEIPT","PRESENCE","HISTORY_SYNC","CHAT_PRESENCE","CALL","CONNECTION","LABEL","CONTACT","GROUP","NEWSLETTER","QRCODE"],"webhookUrl":"https://seu-dominio.com/webhook"}' },
  { id: 'disconnect', method: 'POST', path: '/api/instance/disconnect', title: 'Desconectar', description: 'Desconecta sem remover o pareamento existente.' },
  { id: 'logout', method: 'DELETE', path: '/api/instance/logout', title: 'Logout', description: 'Desvincula a conta e apaga o pareamento local.' },
  { id: 'pair', method: 'POST', path: '/api/instance/pair', title: 'Código de pareamento', description: 'Gera um código temporário usando telefone internacional sem +.', body: '{"phone":"5511999999999"}' },
  { id: 'proxy', method: 'DELETE', path: '/api/instance/proxy', title: 'Remover proxy', description: 'Limpa o proxy em uso e reconecta o cliente quando necessário.' },
  { id: 'qr', method: 'GET', path: '/api/instance/qr?format=base64', title: 'Obter QR Code', description: 'Retorna PNG ou JSON Base64 com format=base64.' },
  { id: 'status', method: 'GET', path: '/api/instance/status', title: 'Status', description: 'Retorna o estado atual, disponibilidade e validade do QR Code.' },
  { id: 'check', method: 'POST', path: '/api/contacts/check', title: 'Verificar contatos', description: 'Verifica até 100 telefones e retorna quais possuem WhatsApp.', body: '{"phones":["5511999999999","5511888888888"]}' },
]

type MessageActionEndpointInfo = {
  id: MessageActionEndpoint
  method: 'GET' | 'POST'
  path: string
  examplePath: string
  title: string
  description: string
  json?: string
  php?: string
  python?: string
}

const messageActionEndpoints: MessageActionEndpointInfo[] = [
  { id: 'delete-message', method: 'POST', path: '/api/messages/delete', examplePath: '/api/messages/delete', title: 'Excluir para todos', description: 'Revoga uma mensagem enviada. Em grupos, participant permite excluir mensagem de outro membro quando a conta for administradora.', json: '{"chat":"5511999999999","messageId":"3EB0ABC123"}', php: "'chat' => '5511999999999',\n        'messageId' => '3EB0ABC123',", python: "'chat': '5511999999999',\n        'messageId': '3EB0ABC123'," },
  { id: 'edit-message', method: 'POST', path: '/api/messages/edit', examplePath: '/api/messages/edit', title: 'Editar mensagem', description: 'Edita uma mensagem de texto enviada dentro da janela permitida pelo WhatsApp.', json: '{"chat":"5511999999999","messageId":"3EB0ABC123","message":"Texto corrigido"}', php: "'chat' => '5511999999999',\n        'messageId' => '3EB0ABC123',\n        'message' => 'Texto corrigido',", python: "'chat': '5511999999999',\n        'messageId': '3EB0ABC123',\n        'message': 'Texto corrigido'," },
  { id: 'mark-read', method: 'POST', path: '/api/messages/read', examplePath: '/api/messages/read', title: 'Marcar como lida', description: 'Envia confirmação de leitura para até 100 IDs do mesmo remetente. Em grupos, informe participant.', json: '{"chat":"5511999999999","messageIds":["3EB0ABC123"]}', php: "'chat' => '5511999999999',\n        'messageIds' => ['3EB0ABC123'],", python: "'chat': '5511999999999',\n        'messageIds': ['3EB0ABC123']," },
  { id: 'message-status', method: 'GET', path: '/api/messages/{messageID}/status', examplePath: '/api/messages/3EB0ABC123/status', title: 'Status da mensagem', description: 'Retorna o último recibo persistido: sent, delivered, read ou played.' },
  { id: 'archive-chat', method: 'POST', path: '/api/chats/archive', examplePath: '/api/chats/archive', title: 'Arquivar conversa', description: 'Arquiva a conversa. Envie archived=false para desarquivar.', json: '{"chat":"5511999999999","archived":true}', php: "'chat' => '5511999999999',\n        'archived' => true,", python: "'chat': '5511999999999',\n        'archived': True," },
  { id: 'mute-chat', method: 'POST', path: '/api/chats/mute', examplePath: '/api/chats/mute', title: 'Silenciar conversa', description: 'Silencia pelo período em segundos; zero mantém silenciada sem prazo.', json: '{"chat":"5511999999999","durationSeconds":28800}', php: "'chat' => '5511999999999',\n        'durationSeconds' => 28800,", python: "'chat': '5511999999999',\n        'durationSeconds': 28800," },
  { id: 'pin-chat', method: 'POST', path: '/api/chats/pin', examplePath: '/api/chats/pin', title: 'Fixar conversa', description: 'Fixa a conversa na lista do WhatsApp.', json: '{"chat":"5511999999999"}', php: "'chat' => '5511999999999',", python: "'chat': '5511999999999'," },
  { id: 'unpin-chat', method: 'POST', path: '/api/chats/unpin', examplePath: '/api/chats/unpin', title: 'Desafixar conversa', description: 'Remove a conversa da lista de fixadas.', json: '{"chat":"5511999999999"}', php: "'chat' => '5511999999999',", python: "'chat': '5511999999999'," },
]

type OrganizationEndpointInfo = {
  id: OrganizationEndpoint
  category: 'Newsletter' | 'Etiqueta' | 'Comunidade'
  method: 'GET' | 'POST' | 'PATCH' | 'DELETE'
  path: string
  title: string
  description: string
  json?: string
  php?: string
  python?: string
}

const organizationEndpoints: OrganizationEndpointInfo[] = [
  { id: 'create-newsletter', category: 'Newsletter', method: 'POST', path: '/api/newsletters', title: 'Criar newsletter', description: 'Cria um canal do WhatsApp com nome e descrição.', json: '{"name":"Novidades Wirely","description":"Atualizações e novidades"}', php: "'name' => 'Novidades Wirely',\n        'description' => 'Atualizações e novidades',", python: "'name': 'Novidades Wirely',\n        'description': 'Atualizações e novidades'," },
  { id: 'get-newsletter', category: 'Newsletter', method: 'POST', path: '/api/newsletters/info', title: 'Consultar newsletter', description: 'Consulta metadados de um canal pelo JID.', json: '{"jid":"120363000000000000@newsletter"}', php: "'jid' => '120363000000000000@newsletter',", python: "'jid': '120363000000000000@newsletter'," },
  { id: 'invite-newsletter', category: 'Newsletter', method: 'POST', path: '/api/newsletters/invite', title: 'Consultar convite', description: 'Resolve um canal pelo código ou link de convite.', json: '{"key":"https://whatsapp.com/channel/SEU_CODIGO"}', php: "'key' => 'https://whatsapp.com/channel/SEU_CODIGO',", python: "'key': 'https://whatsapp.com/channel/SEU_CODIGO'," },
  { id: 'list-newsletters', category: 'Newsletter', method: 'GET', path: '/api/newsletters', title: 'Listar newsletters', description: 'Lista os canais que a instância acompanha.' },
  { id: 'newsletter-messages', category: 'Newsletter', method: 'POST', path: '/api/newsletters/messages', title: 'Mensagens da newsletter', description: 'Busca até 100 mensagens; before permite paginação pelo ID do servidor.', json: '{"jid":"120363000000000000@newsletter","count":20}', php: "'jid' => '120363000000000000@newsletter',\n        'count' => 20,", python: "'jid': '120363000000000000@newsletter',\n        'count': 20," },
  { id: 'subscribe-newsletter', category: 'Newsletter', method: 'POST', path: '/api/newsletters/subscribe', title: 'Assinar newsletter', description: 'Faz a conta conectada acompanhar o canal.', json: '{"jid":"120363000000000000@newsletter"}', php: "'jid' => '120363000000000000@newsletter',", python: "'jid': '120363000000000000@newsletter'," },
  { id: 'add-chat-label', category: 'Etiqueta', method: 'POST', path: '/api/labels/chat', title: 'Adicionar à conversa', description: 'Associa uma etiqueta a uma conversa.', json: '{"chat":"5511999999999","labelId":"1"}', php: "'chat' => '5511999999999',\n        'labelId' => '1',", python: "'chat': '5511999999999',\n        'labelId': '1'," },
  { id: 'edit-label', category: 'Etiqueta', method: 'PATCH', path: '/api/labels/{labelID}', title: 'Editar etiqueta', description: 'Cria, renomeia, recolore ou exclui logicamente uma etiqueta.', json: '{"name":"Urgente","color":3,"deleted":false}', php: "'name' => 'Urgente',\n        'color' => 3,\n        'deleted' => false,", python: "'name': 'Urgente',\n        'color': 3,\n        'deleted': False," },
  { id: 'add-message-label', category: 'Etiqueta', method: 'POST', path: '/api/labels/message', title: 'Adicionar à mensagem', description: 'Associa uma etiqueta a uma mensagem específica.', json: '{"chat":"5511999999999","messageId":"3EB0ABC123","labelId":"1"}', php: "'chat' => '5511999999999',\n        'messageId' => '3EB0ABC123',\n        'labelId' => '1',", python: "'chat': '5511999999999',\n        'messageId': '3EB0ABC123',\n        'labelId': '1'," },
  { id: 'remove-chat-label', category: 'Etiqueta', method: 'DELETE', path: '/api/labels/chat', title: 'Remover da conversa', description: 'Remove a associação entre etiqueta e conversa.', json: '{"chat":"5511999999999","labelId":"1"}', php: "'chat' => '5511999999999',\n        'labelId' => '1',", python: "'chat': '5511999999999',\n        'labelId': '1'," },
  { id: 'remove-message-label', category: 'Etiqueta', method: 'DELETE', path: '/api/labels/message', title: 'Remover da mensagem', description: 'Remove a associação entre etiqueta e mensagem.', json: '{"chat":"5511999999999","messageId":"3EB0ABC123","labelId":"1"}', php: "'chat' => '5511999999999',\n        'messageId' => '3EB0ABC123',\n        'labelId' => '1',", python: "'chat': '5511999999999',\n        'messageId': '3EB0ABC123',\n        'labelId': '1'," },
  { id: 'add-community-groups', category: 'Comunidade', method: 'POST', path: '/api/communities/groups', title: 'Vincular grupos', description: 'Adiciona grupos existentes como participantes da comunidade.', json: '{"communityJid":"120363000000000001@g.us","groupJids":["120363000000000002@g.us"]}', php: "'communityJid' => '120363000000000001@g.us',\n        'groupJids' => ['120363000000000002@g.us'],", python: "'communityJid': '120363000000000001@g.us',\n        'groupJids': ['120363000000000002@g.us']," },
  { id: 'create-community', category: 'Comunidade', method: 'POST', path: '/api/communities', title: 'Criar comunidade', description: 'Cria uma comunidade e seu grupo de anúncios.', json: '{"name":"Comunidade Wirely"}', php: "'name' => 'Comunidade Wirely',", python: "'name': 'Comunidade Wirely'," },
  { id: 'remove-community-groups', category: 'Comunidade', method: 'DELETE', path: '/api/communities/groups', title: 'Desvincular grupos', description: 'Remove grupos participantes da comunidade sem excluir os grupos.', json: '{"communityJid":"120363000000000001@g.us","groupJids":["120363000000000002@g.us"]}', php: "'communityJid' => '120363000000000001@g.us',\n        'groupJids' => ['120363000000000002@g.us'],", python: "'communityJid': '120363000000000001@g.us',\n        'groupJids': ['120363000000000002@g.us']," },
]

const endpoints: { id: Endpoint; title: string; description: string }[] = [
  { id: 'text', title: 'Texto', description: 'Mensagem de até 4.096 caracteres em JSON.' },
  { id: 'image', title: 'Imagem', description: 'Use type=image com legenda opcional.' },
  { id: 'video', title: 'Vídeo', description: 'Use type=video com legenda opcional.' },
  { id: 'audio', title: 'Áudio', description: 'Use type=audio; voice=true envia como voz OGG/Opus.' },
  { id: 'document', title: 'Documento', description: 'Use type=document; o nome original do arquivo é preservado.' },
  { id: 'sticker', title: 'Figurinha', description: 'Use type=sticker com um arquivo WebP sem legenda.' },
  { id: 'location', title: 'Localização', description: 'Latitude e longitude, com nome e endereço opcionais.' },
  { id: 'contact', title: 'Contato', description: 'O Wirely monta um vCard seguro usando nome, telefone e organização.' },
  { id: 'poll', title: 'Enquete', description: 'De 2 a 12 opções e escolha única ou múltipla.' },
  { id: 'reaction', title: 'Reação', description: 'Reaja pelo ID da mensagem; reação vazia remove a reação atual.' },
]

const languages: { id: Language; label: string }[] = [
  { id: 'curl', label: 'cURL' }, { id: 'laravel', label: 'Laravel' },
  { id: 'node', label: 'Node.js' }, { id: 'python', label: 'Python' },
]

type GroupEndpointInfo = {
  id: GroupEndpoint
  method: 'GET' | 'POST' | 'PATCH'
  path: string
  examplePath: string
  title: string
  description: string
  json?: string
  php?: string
  python?: string
}

const groupEndpoints: GroupEndpointInfo[] = [
  {
    id: 'list', method: 'GET', path: '/api/groups', examplePath: '/api/groups', title: 'Listar grupos',
    description: 'Retorna os grupos dos quais a instância conectada participa, incluindo participantes.',
  },
  {
    id: 'create', method: 'POST', path: '/api/groups', examplePath: '/api/groups', title: 'Criar grupo',
    description: 'Cria um grupo e adiciona os números informados como participantes.',
    json: '{"name":"Equipe Wirely","participants":["5511999999999","5511888888888"]}',
    php: "'name' => 'Equipe Wirely',\n        'participants' => ['5511999999999', '5511888888888'],",
    python: "'name': 'Equipe Wirely',\n        'participants': ['5511999999999', '5511888888888'],",
  },
  {
    id: 'join', method: 'POST', path: '/api/groups/join', examplePath: '/api/groups/join', title: 'Entrar por convite',
    description: 'Adiciona a instância a um grupo usando o link ou o código do convite.',
    json: '{"code":"https://chat.whatsapp.com/SEU_CODIGO"}',
    php: "'code' => 'https://chat.whatsapp.com/SEU_CODIGO',",
    python: "'code': 'https://chat.whatsapp.com/SEU_CODIGO',",
  },
  {
    id: 'details', method: 'GET', path: '/api/groups/{groupJID}', examplePath: '/api/groups/120363012345678901@g.us', title: 'Consultar grupo',
    description: 'Consulta nome, descrição, configurações e participantes pelo JID do grupo.',
  },
  {
    id: 'rename', method: 'PATCH', path: '/api/groups/{groupJID}', examplePath: '/api/groups/120363012345678901@g.us', title: 'Alterar nome',
    description: 'Altera o nome do grupo. A conta conectada precisa ter permissão no WhatsApp.',
    json: '{"name":"Novo nome do grupo"}',
    php: "'name' => 'Novo nome do grupo',",
    python: "'name': 'Novo nome do grupo',",
  },
  {
    id: 'participants', method: 'POST', path: '/api/groups/{groupJID}/participants', examplePath: '/api/groups/120363012345678901@g.us/participants', title: 'Participantes',
    description: 'Use action add, remove, promote ou demote para gerenciar participantes e administradores.',
    json: '{"action":"add","participants":["5511999999999"]}',
    php: "'action' => 'add', // add, remove, promote ou demote\n        'participants' => ['5511999999999'],",
    python: "'action': 'add',  # add, remove, promote ou demote\n        'participants': ['5511999999999'],",
  },
  {
    id: 'invite', method: 'GET', path: '/api/groups/{groupJID}/invite', examplePath: '/api/groups/120363012345678901@g.us/invite', title: 'Obter convite',
    description: 'Retorna o link de convite atual do grupo.',
  },
  {
    id: 'rotate-invite', method: 'POST', path: '/api/groups/{groupJID}/invite', examplePath: '/api/groups/120363012345678901@g.us/invite', title: 'Renovar convite',
    description: 'Revoga o convite anterior e gera um novo link para o grupo.',
  },
]

type ProfileEndpointInfo = {
  id: ProfileEndpoint
  method: 'GET' | 'PATCH' | 'PUT' | 'DELETE'
  path: string
  title: string
  description: string
  json?: string
  php?: string
  python?: string
  photo?: boolean
}

const profileEndpoints: ProfileEndpointInfo[] = [
  {
    id: 'get-profile', method: 'GET', path: '/api/profile', title: 'Consultar perfil',
    description: 'Retorna JID, nome, recado e dados da foto da conta conectada.',
  },
  {
    id: 'update-profile', method: 'PATCH', path: '/api/profile', title: 'Alterar perfil',
    description: 'Altera o nome (até 25 caracteres) e/ou o recado (até 139 caracteres).',
    json: '{"name":"Atendimento Wirely","about":"Atendimento disponível"}',
    php: "'name' => 'Atendimento Wirely',\n        'about' => 'Atendimento disponível',",
    python: "'name': 'Atendimento Wirely',\n        'about': 'Atendimento disponível',",
  },
  {
    id: 'set-photo', method: 'PUT', path: '/api/profile/photo', title: 'Definir foto',
    description: 'Envia uma imagem JPEG de até 5 MB como foto do perfil.',
    photo: true,
  },
  {
    id: 'delete-photo', method: 'DELETE', path: '/api/profile/photo', title: 'Remover foto',
    description: 'Remove a foto atual do perfil da conta conectada.',
  },
  {
    id: 'get-privacy', method: 'GET', path: '/api/profile/privacy', title: 'Consultar privacidade',
    description: 'Retorna as configurações de visibilidade, leitura, grupos, chamadas e mensagens.',
  },
  {
    id: 'update-privacy', method: 'PATCH', path: '/api/profile/privacy', title: 'Alterar privacidade',
    description: 'Altera uma configuração por vez e retorna o conjunto atualizado.',
    json: '{"setting":"profile","value":"contacts"}',
    php: "'setting' => 'profile',\n        'value' => 'contacts',",
    python: "'setting': 'profile',\n        'value': 'contacts',",
  },
]

const mediaExamples: Record<MediaEndpoint, { path: string; name: string; mime: string; caption: string }> = {
  image: { path: './photo.jpg', name: 'photo.jpg', mime: 'image/jpeg', caption: 'Imagem enviada pelo Wirely' },
  video: { path: './video.mp4', name: 'video.mp4', mime: 'video/mp4', caption: 'Vídeo enviado pelo Wirely' },
  audio: { path: './voice.ogg', name: 'voice.ogg', mime: 'audio/ogg', caption: '' },
  document: { path: './invoice.pdf', name: 'invoice.pdf', mime: 'application/pdf', caption: 'Fatura' },
  sticker: { path: './sticker.webp', name: 'sticker.webp', mime: 'image/webp', caption: '' },
}

const jsonExamples: Record<JSONEndpoint, { json: string; php: string; python: string }> = {
  text: {
    json: '{"recipient":"5511999999999","message":"Olá pelo Wirely"}',
    php: "'recipient' => '5511999999999',\n        'message' => 'Olá pelo Wirely',",
    python: "'recipient': '5511999999999',\n        'message': 'Olá pelo Wirely',",
  },
  location: {
    json: '{"recipient":"5511999999999","latitude":-23.5505,"longitude":-46.6333,"name":"Praça da Sé","address":"São Paulo - SP"}',
    php: "'recipient' => '5511999999999',\n        'latitude' => -23.5505,\n        'longitude' => -46.6333,\n        'name' => 'Praça da Sé',\n        'address' => 'São Paulo - SP',",
    python: "'recipient': '5511999999999',\n        'latitude': -23.5505,\n        'longitude': -46.6333,\n        'name': 'Praça da Sé',\n        'address': 'São Paulo - SP',",
  },
  contact: {
    json: '{"recipient":"5511999999999","fullName":"Maria Silva","organization":"Wirely","phone":"5511888888888"}',
    php: "'recipient' => '5511999999999',\n        'fullName' => 'Maria Silva',\n        'organization' => 'Wirely',\n        'phone' => '5511888888888',",
    python: "'recipient': '5511999999999',\n        'fullName': 'Maria Silva',\n        'organization': 'Wirely',\n        'phone': '5511888888888',",
  },
  poll: {
    json: '{"recipient":"5511999999999","question":"Qual horário você prefere?","choices":["09:00","14:00"],"maxAnswer":1}',
    php: "'recipient' => '5511999999999',\n        'question' => 'Qual horário você prefere?',\n        'choices' => ['09:00', '14:00'],\n        'maxAnswer' => 1,",
    python: "'recipient': '5511999999999',\n        'question': 'Qual horário você prefere?',\n        'choices': ['09:00', '14:00'],\n        'maxAnswer': 1,",
  },
  reaction: {
    json: '{"recipient":"5511999999999","messageId":"3EB0...","reaction":"👍","fromMe":false}',
    php: "'recipient' => '5511999999999',\n        'messageId' => '3EB0...',\n        'reaction' => '👍',\n        'fromMe' => false,",
    python: "'recipient': '5511999999999',\n        'messageId': '3EB0...',\n        'reaction': '👍',\n        'fromMe': False,",
  },
}

function isMediaEndpoint(endpoint: Endpoint): endpoint is MediaEndpoint {
  return ['image', 'video', 'audio', 'document', 'sticker'].includes(endpoint)
}

function snippet(language: Language, endpoint: Endpoint, origin: string, token: string) {
  const url = `${origin}/api/send/${isMediaEndpoint(endpoint) ? 'media' : endpoint}`
  const credential = token || 'wly_SEU_TOKEN'
  const presence = endpoint === 'audio' ? 'recording' : 'composing'
  if (!isMediaEndpoint(endpoint)) {
    const example = jsonExamples[endpoint]
    const json = example.json.replace('{', '{"options":{"presence":"composing","delay":2000},')
    if (language === 'curl') return `curl -X POST '${url}' \\
  -H 'Authorization: Bearer ${credential}' \\
  -H 'Content-Type: application/json' \\
  -d '${json}'`
    if (language === 'laravel') return `use Illuminate\\Support\\Facades\\Http;

$response = Http::withToken('${credential}')
    ->post('${url}', [
        'options' => ['presence' => 'composing', 'delay' => 2000],
        ${example.php}
    ]);

$message = $response->throw()->json();`
    if (language === 'node') return `const response = await fetch('${url}', {
  method: 'POST',
  headers: {
    Authorization: 'Bearer ${credential}',
    'Content-Type': 'application/json',
  },
  body: JSON.stringify(${json}),
});

if (!response.ok) throw new Error(await response.text());
console.log(await response.json());`
    return `import requests

response = requests.post(
    '${url}',
    headers={'Authorization': 'Bearer ${credential}'},
    json={
        'options': {'presence': 'composing', 'delay': 2000},
        ${example.python}
    },
    timeout=30,
)
response.raise_for_status()
print(response.json())`
  }

  const media = mediaExamples[endpoint]
  const fields = [`recipient=5511999999999`, `type=${endpoint}`, `options={"presence":"${presence}","delay":2000}`]
  if (endpoint === 'audio') fields.push('voice=true')
  else if (media.caption) fields.push(`caption=${media.caption}`)
  fields.push(`file=@${media.path};type=${media.mime}`)

  if (language === 'curl') {
    const continuation = ' \\'
    const form = fields.map((field, index) => `  -F '${field}'${index < fields.length - 1 ? continuation : ''}`).join('\n')
    return `curl -X POST '${url}' \\
  -H 'Authorization: Bearer ${credential}' \\
${form}`
  }
  if (language === 'laravel') {
    const options = [`        'recipient' => '5511999999999',`, `        'type' => '${endpoint}',`, `        'options' => json_encode(['presence' => '${presence}', 'delay' => 2000]),`]
    if (endpoint === 'audio') options.push("        'voice' => true,")
    else if (media.caption) options.push(`        'caption' => '${media.caption}',`)
    return `use Illuminate\\Support\\Facades\\Http;

$response = Http::withToken('${credential}')
    ->attach('file', fopen('${media.path}', 'r'), '${media.name}')
    ->post('${url}', [
${options.join('\n')}
    ]);

$message = $response->throw()->json();`
  }
  if (language === 'node') {
    const option = endpoint === 'audio'
      ? "form.append('voice', 'true');"
      : media.caption ? `form.append('caption', '${media.caption}');` : ''
    return `import { readFile } from 'node:fs/promises';

const form = new FormData();
form.append('recipient', '5511999999999');
form.append('type', '${endpoint}');
form.append('options', JSON.stringify({ presence: '${presence}', delay: 2000 }));
${option ? option + '\n' : ''}form.append('file', new Blob([await readFile('${media.path}')], {
  type: '${media.mime}',
}), '${media.name}');

const response = await fetch('${url}', {
  method: 'POST',
  headers: { Authorization: 'Bearer ${credential}' },
  body: form,
});

if (!response.ok) throw new Error(await response.text());
console.log(await response.json());`
  }

  const extraPython = endpoint === 'audio'
    ? ", 'voice': 'true'"
    : media.caption ? `, 'caption': '${media.caption}'` : ''
  return `import requests

with open('${media.path}', 'rb') as file:
    response = requests.post(
        '${url}',
        headers={'Authorization': 'Bearer ${credential}'},
        data={'recipient': '5511999999999', 'type': '${endpoint}', 'options': '{"presence":"${presence}","delay":2000}'${extraPython}},
        files={'file': ('${media.name}', file, '${media.mime}')},
        timeout=120,
    )

response.raise_for_status()
print(response.json())`
}

function organizationSnippet(language: Language, endpoint: OrganizationEndpoint, origin: string, token: string) {
  const info = organizationEndpoints.find((item) => item.id === endpoint)!
  const url = `${origin}${info.path.replace('{labelID}', '1')}`
  const credential = token || 'wly_SEU_TOKEN'
  const hasBody = Boolean(info.json)
  if (language === 'curl') {
    const body = hasBody ? ` \\\n  -H 'Content-Type: application/json' \\\n  -d '${info.json}'` : ''
    return `curl -X ${info.method} '${url}' \\\n  -H 'Authorization: Bearer ${credential}'${body}`
  }
  if (language === 'laravel') {
    const method = info.method.toLowerCase()
    const args = hasBody ? `, [\n        ${info.php ?? ''}\n    ]` : ''
    return `use Illuminate\\Support\\Facades\\Http;

$response = Http::withToken('${credential}')
    ->${method}('${url}'${args});

$data = $response->throw()->json();`
  }
  if (language === 'node') {
    const contentType = hasBody ? `\n    'Content-Type': 'application/json',` : ''
    const body = hasBody ? `\n  body: JSON.stringify(${info.json}),` : ''
    return `const response = await fetch('${url}', {
  method: '${info.method}',
  headers: {
    Authorization: 'Bearer ${credential}',${contentType}
  },${body}
});

if (!response.ok) throw new Error(await response.text());
console.log(await response.json());`
  }
  const body = hasBody ? `,\n    json={\n        ${info.python ?? ''}\n    }` : ''
  return `import requests

response = requests.${info.method.toLowerCase()}(
    '${url}',
    headers={'Authorization': 'Bearer ${credential}'}${body},
    timeout=30,
)
response.raise_for_status()
print(response.json())`
}

function messageActionSnippet(language: Language, endpoint: MessageActionEndpoint, origin: string, token: string) {
  const info = messageActionEndpoints.find((item) => item.id === endpoint)!
  const url = `${origin}${info.examplePath}`
  const credential = token || 'wly_SEU_TOKEN'
  const hasBody = Boolean(info.json)
  if (language === 'curl') {
    const body = hasBody ? ` \\\n  -H 'Content-Type: application/json' \\\n  -d '${info.json}'` : ''
    return `curl -X ${info.method} '${url}' \\\n  -H 'Authorization: Bearer ${credential}'${body}`
  }
  if (language === 'laravel') {
    const method = info.method === 'GET' ? 'get' : 'post'
    const args = hasBody ? `, [\n        ${info.php ?? ''}\n    ]` : ''
    return `use Illuminate\\Support\\Facades\\Http;

$response = Http::withToken('${credential}')
    ->${method}('${url}'${args});

$data = $response->throw()->json();`
  }
  if (language === 'node') {
    const headers = hasBody ? `Authorization: 'Bearer ${credential}',\n    'Content-Type': 'application/json',` : `Authorization: 'Bearer ${credential}',`
    const body = hasBody ? `\n  body: JSON.stringify(${info.json}),` : ''
    return `const response = await fetch('${url}', {
  method: '${info.method}',
  headers: {
    ${headers}
  },${body}
});

if (!response.ok) throw new Error(await response.text());
console.log(await response.json());`
  }
  const body = hasBody ? `,\n    json={\n        ${info.python ?? ''}\n    }` : ''
  return `import requests

response = requests.${info.method.toLowerCase()}(
    '${url}',
    headers={'Authorization': 'Bearer ${credential}'}${body},
    timeout=30,
)
response.raise_for_status()
print(response.json())`
}

function groupSnippet(language: Language, endpoint: GroupEndpoint, origin: string, token: string) {
  const info = groupEndpoints.find((item) => item.id === endpoint)!
  const url = `${origin}${info.examplePath}`
  const credential = token || 'wly_SEU_TOKEN'
  const hasBody = Boolean(info.json)

  if (language === 'curl') {
    const headers = hasBody
      ? `  -H 'Authorization: Bearer ${credential}' \\\n  -H 'Content-Type: application/json' \\\n  -d '${info.json}'`
      : `  -H 'Authorization: Bearer ${credential}'`
    return `curl -X ${info.method} '${url}' \\\n${headers}`
  }

  if (language === 'laravel') {
    const method = info.method.toLowerCase()
    const request = hasBody
      ? `    ->${method}('${url}', [\n        ${info.php ?? ''}\n    ]);`
      : `    ->${method}('${url}');`
    return `use Illuminate\\Support\\Facades\\Http;

$response = Http::withToken('${credential}')
${request}

$data = $response->throw()->noContent() ? null : $response->json();`
  }

  if (language === 'node') {
    const contentType = hasBody ? ",\n    'Content-Type': 'application/json'" : ''
    const body = hasBody ? `,\n  body: JSON.stringify(${info.json})` : ''
    return `const response = await fetch('${url}', {
  method: '${info.method}',
  headers: {
    Authorization: 'Bearer ${credential}'${contentType}
  }${body},
});

if (!response.ok) throw new Error(await response.text());
if (response.status !== 204) console.log(await response.json());`
  }

  const payload = hasBody ? `,\n    json={\n        ${info.python ?? ''}\n    }` : ''
  return `import requests

response = requests.${info.method.toLowerCase()}(
    '${url}',
    headers={'Authorization': 'Bearer ${credential}'}${payload},
    timeout=30,
)
response.raise_for_status()
if response.content:
    print(response.json())`
}

function profileSnippet(language: Language, endpoint: ProfileEndpoint, origin: string, token: string) {
  const info = profileEndpoints.find((item) => item.id === endpoint)!
  const url = `${origin}${info.path}`
  const credential = token || 'wly_SEU_TOKEN'

  if (info.photo) {
    if (language === 'curl') return `curl -X PUT '${url}' \\\n  -H 'Authorization: Bearer ${credential}' \\\n  -F 'file=@./profile.jpg;type=image/jpeg'`
    if (language === 'laravel') return `use Illuminate\\Support\\Facades\\Http;

$response = Http::withToken('${credential}')
    ->attach('file', fopen('./profile.jpg', 'r'), 'profile.jpg')
    ->put('${url}');

$data = $response->throw()->json();`
    if (language === 'node') return `import { readFile } from 'node:fs/promises';

const form = new FormData();
form.append('file', new Blob([await readFile('./profile.jpg')], {
  type: 'image/jpeg',
}), 'profile.jpg');

const response = await fetch('${url}', {
  method: 'PUT',
  headers: { Authorization: 'Bearer ${credential}' },
  body: form,
});

if (!response.ok) throw new Error(await response.text());
console.log(await response.json());`
    return `import requests

with open('./profile.jpg', 'rb') as file:
    response = requests.put(
        '${url}',
        headers={'Authorization': 'Bearer ${credential}'},
        files={'file': ('profile.jpg', file, 'image/jpeg')},
        timeout=120,
    )

response.raise_for_status()
print(response.json())`
  }

  const hasBody = Boolean(info.json)
  if (language === 'curl') {
    const headers = hasBody
      ? `  -H 'Authorization: Bearer ${credential}' \\\n  -H 'Content-Type: application/json' \\\n  -d '${info.json}'`
      : `  -H 'Authorization: Bearer ${credential}'`
    return `curl -X ${info.method} '${url}' \\\n${headers}`
  }

  if (language === 'laravel') {
    const method = info.method.toLowerCase()
    const request = hasBody
      ? `    ->${method}('${url}', [\n        ${info.php ?? ''}\n    ]);`
      : `    ->${method}('${url}');`
    return `use Illuminate\\Support\\Facades\\Http;

$response = Http::withToken('${credential}')
${request}

$data = $response->throw()->noContent() ? null : $response->json();`
  }

  if (language === 'node') {
    const contentType = hasBody ? ",\n    'Content-Type': 'application/json'" : ''
    const body = hasBody ? `,\n  body: JSON.stringify(${info.json})` : ''
    return `const response = await fetch('${url}', {
  method: '${info.method}',
  headers: {
    Authorization: 'Bearer ${credential}'${contentType}
  }${body},
});

if (!response.ok) throw new Error(await response.text());
if (response.status !== 204) console.log(await response.json());`
  }

  const payload = hasBody ? `,\n    json={\n        ${info.python ?? ''}\n    }` : ''
  return `import requests

response = requests.${info.method.toLowerCase()}(
    '${url}',
    headers={'Authorization': 'Bearer ${credential}'}${payload},
    timeout=30,
)
response.raise_for_status()
if response.content:
    print(response.json())`
}

function queueSnippet(origin: string, token: string) {
  const credential = token || 'wly_SEU_TOKEN'
  return `curl -X POST '${origin}/api/queue/text' \\
  -H 'Authorization: Bearer ${credential}' \\
  -H 'Idempotency-Key: pedido-123' \\
  -H 'Content-Type: application/json' \\
  -d '{"recipient":"5511999999999","message":"Olá pela fila"}'

# Imagem, vídeo, áudio, documento e figurinha usam /api/queue/media:
curl -X POST '${origin}/api/queue/media' \\
  -H 'Authorization: Bearer ${credential}' \\
  -H 'Idempotency-Key: midia-123' \\
  -F 'recipient=5511999999999' \\
  -F 'type=video' \\
  -F 'file=@./video.mp4;type=video/mp4'

# A resposta 202 informa o ID do job. Consulte depois:
curl '${origin}/api/queue/job_ID' \\
  -H 'Authorization: Bearer ${credential}'`
}

function instanceSnippetBase(language: Language, endpoint: InstanceEndpoint, origin: string, token: string, instanceID: string) {
  const info = instanceEndpoints.find((item) => item.id === endpoint)!
  const credential = token || 'wly_SEU_TOKEN'
  const path = info.path.replace('{id}', instanceID || 'ID_DA_INSTANCIA')
  const url = `${origin}${path}`
  const body = info.body
  if (endpoint === 'login') {
    if (language === 'curl') return `curl -X POST '${url}' \\
  -H 'Content-Type: application/json' \\
  -c wirely.cookies \\
  -d '${body}'`
    if (language === 'laravel') return `use Illuminate\\Support\\Facades\\Http;

$login = Http::post('${url}', [
    'username' => 'admin',
    'password' => 'SUA_SENHA',
]);

$login->throw();
$session = $login->cookies()->getCookieByName('wirely_session')?->getValue();`
    if (language === 'node') return `const response = await fetch('${url}', {
  method: 'POST',
  credentials: 'include',
  headers: { 'Content-Type': 'application/json' },
  body: JSON.stringify({ username: 'admin', password: 'SUA_SENHA' }),
});

if (!response.ok) throw new Error(await response.text());
console.log(await response.json());`
    return `import requests

session = requests.Session()
response = session.post(
    '${url}',
    json={'username': 'admin', 'password': 'SUA_SENHA'},
    timeout=30,
)
response.raise_for_status()
print(response.json())

# Reutilize session nas rotas administrativas.`
  }
  if (language === 'curl') {
    const auth = info.admin ? `  -b 'wirely.cookies'` : `  -H 'Authorization: Bearer ${credential}'`
    const payload = body ? ` \\\n+  -H 'Content-Type: application/json' \\\n+  -d '${body}'` : ''
    return `${info.admin ? '# Requer sessão administrativa salva em wirely.cookies\n' : ''}curl -X ${info.method} '${url}' \\\n+${auth}${payload}`
  }
  if (language === 'laravel') {
    const auth = info.admin
      ? `Http::withCookies(['wirely_session' => 'SUA_SESSAO'], parse_url('${origin}', PHP_URL_HOST))`
      : `Http::withToken('${credential}')`
    const payload = body ? `, ${body.replaceAll(':', ' =>').replaceAll('{', '[').replaceAll('}', ']').replaceAll('"', "'")}` : ''
    return `use Illuminate\\Support\\Facades\\Http;

$response = ${auth}
    ->${info.method.toLowerCase()}('${url}'${payload});

$data = $response->throw()->noContent() ? null : $response->json();`
  }
  if (language === 'node') {
    const content = body ? `,
    'Content-Type': 'application/json'` : ''
    const bodyLine = body ? `,
  body: JSON.stringify(${body})` : ''
    const auth = info.admin ? `Cookie: 'wirely_session=SUA_SESSAO'` : `Authorization: 'Bearer ${credential}'`
    return `const response = await fetch('${url}', {
  method: '${info.method}',
  headers: { ${auth}${content} }${bodyLine},
});

if (!response.ok) throw new Error(await response.text());
if (response.status !== 204) console.log(await response.json());`
  }
  const auth = info.admin ? `cookies={'wirely_session': 'SUA_SESSAO'}` : `headers={'Authorization': 'Bearer ${credential}'}`
  const payload = body ? `,
    json=${body.replaceAll('true', 'True').replaceAll('false', 'False')}` : ''
  return `import requests

response = requests.${info.method.toLowerCase()}(
    '${url}',
    ${auth}${payload},
    timeout=30,
)
response.raise_for_status()
if response.content:
    print(response.json())`
}

function instanceSnippet(language: Language, endpoint: InstanceEndpoint, origin: string, token: string, instanceID: string) {
  if (language !== 'curl') return instanceSnippetBase(language, endpoint, origin, token, instanceID)
  const info = instanceEndpoints.find((item) => item.id === endpoint)!
  const credential = token || 'wly_SEU_TOKEN'
  const url = `${origin}${info.path.replace('{id}', instanceID || 'ID_DA_INSTANCIA')}`
  if (endpoint === 'login') return `curl -X POST '${url}' \\
  -H 'Content-Type: application/json' \\
  -c wirely.cookies \\
  -d '${info.body}'`
  const login = info.admin ? `# A senha é enviada somente no login:
curl -X POST '${origin}/api/auth/login' \\
  -H 'Content-Type: application/json' \\
  -c wirely.cookies \\
  -d '{"username":"admin","password":"SUA_SENHA"}'

` : ''
  const auth = info.admin
    ? `  -b wirely.cookies`
    : `  -H 'Authorization: Bearer ${credential}'`
  const payload = info.body ? ` \\
  -H 'Content-Type: application/json' \\
  -d '${info.body}'` : ''
  return `${login}curl -X ${info.method} '${url}' \\
${auth}${payload}`
}

const receivedMediaEvent = JSON.stringify({
  id: 'evt_...', event: 'message.received', instanceId: 'instance-id', timestamp: '2026-09-17T12:00:00Z',
  data: {
    id: '3EB0ABC', from: '5511999999999@s.whatsapp.net', chat: '5511999999999@s.whatsapp.net',
    fromMe: false, isGroup: false, type: 'image', text: 'Comprovante',
    media: {
      available: true, type: 'image', mimetype: 'image/jpeg', fileName: 'media-3EB0ABC.jpg',
      size: 48231, downloadUrl: '/api/messages/3EB0ABC/media', caption: 'Comprovante',
      savedAt: '2026-09-17T12:00:00Z',
    },
  },
}, null, 2)

function receivedMediaSnippet(language: Language, origin: string, token: string) {
  const credential = token || 'wly_SEU_TOKEN'
  const url = `${origin}/api/messages/3EB0ABC/media`
  if (language === 'curl') return `curl '${url}' \
  -H 'Authorization: Bearer ${credential}' \
  --output received-media.jpg

# Para receber JSON com Base64:
curl '${url}?format=base64' \
  -H 'Authorization: Bearer ${credential}'`
  if (language === 'laravel') return `use Illuminate\\Support\\Facades\\Http;
use Illuminate\\Support\\Facades\\Storage;

$response = Http::withToken('${credential}')->get('${url}');
$response->throw();
Storage::put('received/received-media.jpg', $response->body());

// Alternativa JSON:
$media = Http::withToken('${credential}')
    ->get('${url}?format=base64')
    ->throw()
    ->json();`
  if (language === 'node') return `import { writeFile } from 'node:fs/promises';

const response = await fetch('${url}', {
  headers: { Authorization: 'Bearer ${credential}' },
});
if (!response.ok) throw new Error(await response.text());
await writeFile('./received-media.jpg', Buffer.from(await response.arrayBuffer()));

// Alternativa JSON:
const media = await fetch('${url}?format=base64', {
  headers: { Authorization: 'Bearer ${credential}' },
}).then((result) => result.json());`
  return `import requests

response = requests.get(
    '${url}',
    headers={'Authorization': 'Bearer ${credential}'},
    timeout=120,
)
response.raise_for_status()
with open('./received-media.jpg', 'wb') as file:
    file.write(response.content)

# Alternativa JSON:
media = requests.get(
    '${url}?format=base64',
    headers={'Authorization': 'Bearer ${credential}'},
    timeout=120,
).json()`
}

function webhookSnippet(language: Language, origin: string, token: string) {
  const credential = token || 'wly_SEU_TOKEN'
  const testURL = `${origin}/api/webhook/test`
  const statusURL = `${origin}/api/webhook/jobs/evt_ID_RETORNADO`
  if (language === 'curl') return `# Enfileira um evento webhook.test
curl -X POST '${testURL}' \
  -H 'Authorization: Bearer ${credential}'

# Consulte o eventId retornado
curl '${statusURL}' \
  -H 'Authorization: Bearer ${credential}'

# Histórico de tentativas
curl '${origin}/api/webhook/deliveries?status=failed' \
  -H 'Authorization: Bearer ${credential}'`
  if (language === 'laravel') return `use Illuminate\\Support\\Facades\\Http;

$queued = Http::withToken('${credential}')
    ->post('${testURL}')
    ->throw()
    ->json();

$job = Http::withToken('${credential}')
    ->get('${origin}/api/webhook/jobs/' . $queued['eventId'])
    ->throw()
    ->json();`
  if (language === 'node') return `const headers = { Authorization: 'Bearer ${credential}' };
const queued = await fetch('${testURL}', { method: 'POST', headers })
  .then(async response => {
    if (!response.ok) throw new Error(await response.text());
    return response.json();
  });

const job = await fetch(
  '${origin}/api/webhook/jobs/' + queued.eventId,
  { headers },
).then(response => response.json());`
  return `import requests

headers = {'Authorization': 'Bearer ${credential}'}
queued = requests.post('${testURL}', headers=headers, timeout=30)
queued.raise_for_status()

event_id = queued.json()['eventId']
job = requests.get(
    f'${origin}/api/webhook/jobs/{event_id}',
    headers=headers,
    timeout=30,
)
job.raise_for_status()
print(job.json())`
}

export function DocsModal({ instances, canManage, onClose, onManage }: {
  instances: Instance[]
  canManage: boolean
  onClose: () => void
  onManage: (instance: Instance) => void
}) {
  const dialog = useRef<HTMLDialogElement>(null)
  const preferred = instances.find((item) => item.status === 'connected') ?? instances[0]
  const [instanceID, setInstanceID] = useState(preferred?.id ?? '')
  const [endpoint, setEndpoint] = useState<Endpoint>('text')
  const [groupEndpoint, setGroupEndpoint] = useState<GroupEndpoint>('list')
  const [profileEndpoint, setProfileEndpoint] = useState<ProfileEndpoint>('get-profile')
  const [instanceEndpoint, setInstanceEndpoint] = useState<InstanceEndpoint>('status')
  const [messageActionEndpoint, setMessageActionEndpoint] = useState<MessageActionEndpoint>('delete-message')
  const [organizationEndpoint, setOrganizationEndpoint] = useState<OrganizationEndpoint>('create-newsletter')
  const [language, setLanguage] = useState<Language>('curl')
  const [token, setToken] = useState('')
  const [loadingToken, setLoadingToken] = useState(Boolean(preferred) && canManage)
  const [tokenError, setTokenError] = useState('')
  const [copied, setCopied] = useState(false)
  const [groupCopied, setGroupCopied] = useState(false)
  const [profileCopied, setProfileCopied] = useState(false)
  const [mediaCopied, setMediaCopied] = useState(false)
  const [webhookCopied, setWebhookCopied] = useState(false)
  const [instanceCopied, setInstanceCopied] = useState(false)
  const [messageActionCopied, setMessageActionCopied] = useState(false)
  const [organizationCopied, setOrganizationCopied] = useState(false)
  const selectedInstance = instances.find((item) => item.id === instanceID)
  const code = useMemo(() => snippet(language, endpoint, window.location.origin, token), [language, endpoint, token])
  const groupCode = useMemo(() => groupSnippet(language, groupEndpoint, window.location.origin, token), [language, groupEndpoint, token])
  const profileCode = useMemo(() => profileSnippet(language, profileEndpoint, window.location.origin, token), [language, profileEndpoint, token])
  const queueCode = useMemo(() => queueSnippet(window.location.origin, token), [token])
  const receivedCode = useMemo(() => receivedMediaSnippet(language, window.location.origin, token), [language, token])
  const webhookCode = useMemo(() => webhookSnippet(language, window.location.origin, token), [language, token])
  const instanceCode = useMemo(() => instanceSnippet(language, instanceEndpoint, window.location.origin, token, instanceID), [language, instanceEndpoint, token, instanceID])
  const messageActionCode = useMemo(() => messageActionSnippet(language, messageActionEndpoint, window.location.origin, token), [language, messageActionEndpoint, token])
  const organizationCode = useMemo(() => organizationSnippet(language, organizationEndpoint, window.location.origin, token), [language, organizationEndpoint, token])

  useEffect(() => {
    const element = dialog.current!
    const previousFocus = document.activeElement as HTMLElement | null
    element.showModal()
    return () => { element.close(); previousFocus?.focus() }
  }, [])

  useEffect(() => {
    if (!instanceID || !canManage) return
    const controller = new AbortController()
    request<{ token: string }>(`/api/instances/${instanceID}/token`, { signal: controller.signal })
      .then((value) => setToken(value.token))
      .catch((reason) => setTokenError(reason instanceof Error ? reason.message : 'Falha ao carregar token.'))
      .finally(() => { if (!controller.signal.aborted) setLoadingToken(false) })
    return () => controller.abort()
  }, [canManage, instanceID])

  function changeInstance(id: string) {
    setInstanceID(id); setToken(''); setTokenError(''); setCopied(false); setGroupCopied(false); setProfileCopied(false); setMediaCopied(false); setWebhookCopied(false); setInstanceCopied(false); setMessageActionCopied(false); setOrganizationCopied(false)
    setLoadingToken(Boolean(id) && canManage)
  }

  async function copyCode(value: string, target: 'send' | 'group' | 'profile' | 'media' | 'webhook' | 'instance' | 'message-action' | 'organization') {
    try {
      let done = false
      if (navigator.clipboard?.writeText) {
        try { await navigator.clipboard.writeText(value); done = true } catch { /* HTTP may deny Clipboard API. */ }
      }
      if (!done) {
        const previousFocus = document.activeElement as HTMLElement | null
        const area = document.createElement('textarea')
        area.value = value
        area.setAttribute('readonly', '')
        area.style.position = 'fixed'; area.style.inset = '0 auto auto 0'
        area.style.width = '1px'; area.style.height = '1px'; area.style.opacity = '0'
        ;(dialog.current ?? document.body).appendChild(area)
        area.focus(); area.select(); area.setSelectionRange(0, area.value.length)
        done = document.execCommand('copy')
        area.remove()
        previousFocus?.focus()
      }
      if (!done) throw new Error('copy failed')
      if (target === 'send') setCopied(true)
      else if (target === 'group') setGroupCopied(true)
      else if (target === 'profile') setProfileCopied(true)
      else if (target === 'media') setMediaCopied(true)
      else if (target === 'webhook') setWebhookCopied(true)
      else if (target === 'instance') setInstanceCopied(true)
      else if (target === 'message-action') setMessageActionCopied(true)
      else setOrganizationCopied(true)
      window.setTimeout(() => target === 'send' ? setCopied(false)
        : target === 'group' ? setGroupCopied(false)
          : target === 'profile' ? setProfileCopied(false)
            : target === 'media' ? setMediaCopied(false)
              : target === 'webhook' ? setWebhookCopied(false)
                : target === 'instance' ? setInstanceCopied(false)
                  : target === 'message-action' ? setMessageActionCopied(false) : setOrganizationCopied(false), 1800)
    } catch { setTokenError('Não foi possível copiar automaticamente.') }
  }

  const endpointInfo = endpoints.find((item) => item.id === endpoint)!
  const profileEndpointInfo = profileEndpoints.find((item) => item.id === profileEndpoint)!
  const groupEndpointInfo = groupEndpoints.find((item) => item.id === groupEndpoint)!
  const instanceEndpointInfo = instanceEndpoints.find((item) => item.id === instanceEndpoint)!
  const messageActionEndpointInfo = messageActionEndpoints.find((item) => item.id === messageActionEndpoint)!
  const organizationEndpointInfo = organizationEndpoints.find((item) => item.id === organizationEndpoint)!
  return (
    <dialog ref={dialog} className="docsModal" aria-labelledby="docs-title"
      onCancel={(event) => { event.preventDefault(); onClose() }}>
      <header className="manageHeader">
        <div><p className="eyebrow">REFERÊNCIA DA API</p><h2 id="docs-title">Documentação</h2>
          <span className="docsVersion">OpenAPI 3.1 · Wirely 0.9.0</span></div>
        <button className="closeButton" type="button" aria-label="Fechar documentação" onClick={onClose}>×</button>
      </header>
      <div className="docsBody">
        <section className="docsCredentials" aria-labelledby="docs-instance-label">
          <div><label id="docs-instance-label" htmlFor="docs-instance">Instância dos exemplos</label>
            <select id="docs-instance" value={instanceID} onChange={(event) => changeInstance(event.target.value)}>
              {instances.length === 0 && <option value="">Nenhuma instância criada</option>}
              {instances.map((item) => <option key={item.id} value={item.id}>{item.name} · {item.status === 'connected' ? 'conectada' : 'desconectada'}</option>)}
            </select></div>
          <div className="docsTokenState">
            <span>Bearer Token</span>
            {loadingToken ? <small>Carregando…</small> : token ? <small className="available">Pronto para copiar nos exemplos</small>
              : <small>{canManage ? tokenError || 'Token antigo indisponível' : 'Seu papel não permite visualizar tokens'}</small>}
          </div>
          {canManage && !loadingToken && selectedInstance && !token && <button className="secondaryButton" type="button" onClick={() => onManage(selectedInstance)}>Gerar token</button>}
        </section>

        <section className="docsSection" aria-labelledby="docs-instances-title">
          <div className="docsSectionTitle"><div><h3 id="docs-instances-title">Instâncias</h3>
            <p>Ciclo completo da instância, adaptado à autenticação segura do Wirely.</p></div><span className="docsQueueBadge">13 ROTAS</span></div>
          <div className="docsEndpoints docsGroupEndpoints">
            {instanceEndpoints.map((item) => <button type="button" key={item.id} className={instanceEndpoint === item.id ? 'active' : ''}
              onClick={() => { setInstanceEndpoint(item.id); setInstanceCopied(false) }}>
              <span className={`method-${item.method.toLowerCase()}`}>{item.method}</span><code>{item.path}</code><strong>{item.title}</strong>
            </button>)}
          </div>
          <div className="docsEndpointNote"><strong>{instanceEndpointInfo.title}</strong><span>{instanceEndpointInfo.description}</span></div>
          <div className="docsCodeHeader docsGroupCodeHeader"><div className="docsLanguages" role="tablist" aria-label="Linguagem do exemplo de instância">
            {languages.map((item) => <button key={item.id} type="button" role="tab" aria-selected={language === item.id}
              onClick={() => { setLanguage(item.id); setInstanceCopied(false) }}>{item.label}</button>)}
          </div><button className="secondaryButton" type="button" onClick={() => void copyCode(instanceCode, 'instance')}>{instanceCopied ? 'Copiado ✓' : 'Copiar código'}</button></div>
          <pre className="docsCode docsGroupCode" aria-label={`Exemplo de instância em ${languages.find((item) => item.id === language)?.label}`}><code>{instanceCode}</code></pre>
          <p className="docsSecurity">Use a senha somente em <code>POST /api/auth/login</code>. Listar, criar e excluir reutilizam o cookie da sessão; as demais rotas identificam a instância pelo Bearer Token.</p>
        </section>

        <section className="docsSection">
          <div className="docsSectionTitle"><div><h3>Endpoints de envio</h3><p>Escolha um endpoint para gerar o exemplo.</p></div>
            <a className="secondaryButton" href="/openapi.json" target="_blank" rel="noreferrer">OpenAPI JSON</a></div>
          <div className="docsEndpoints">
            {endpoints.map((item) => <button type="button" key={item.id} className={endpoint === item.id ? 'active' : ''}
              onClick={() => { setEndpoint(item.id); setCopied(false) }}><span>POST</span><code>{isMediaEndpoint(item.id) ? '/api/send/media' : `/api/send/${item.id}`}</code><strong>{item.title}</strong></button>)}
          </div>
          <div className="docsEndpointNote"><strong>{endpointInfo.title}</strong><span>{endpointInfo.description}</span></div>
        </section>

        <section className="docsSection">
          <div className="docsCodeHeader"><div className="docsLanguages" role="tablist" aria-label="Linguagem do exemplo">
            {languages.map((item) => <button key={item.id} type="button" role="tab" aria-selected={language === item.id}
              onClick={() => { setLanguage(item.id); setCopied(false); setGroupCopied(false); setProfileCopied(false); setMediaCopied(false) }}>{item.label}</button>)}
          </div><button className="secondaryButton" type="button" onClick={() => void copyCode(code, 'send')}>{copied ? 'Copiado ✓' : 'Copiar código'}</button></div>
          <pre className="docsCode" aria-label={`Exemplo ${languages.find((item) => item.id === language)?.label}`}><code>{code}</code></pre>
          <p className="docsSecurity">O exemplo usa o token da instância selecionada. Não publique nem envie esse código com o token preenchido.</p>
          <p className="docsDelayNote"><strong>options</strong><span>Envia <code>composing</code> (Digitando…) ou <code>recording</code> (Gravando áudio…), aguarda <code>delay</code> de 0 a 60000 ms e finaliza com <code>paused</code>.</span></p>
        </section>

        <section className="docsSection" aria-labelledby="docs-message-actions-title">
          <div className="docsSectionTitle"><div><h3 id="docs-message-actions-title">Mensagens e conversas</h3>
            <p>Edite, exclua e acompanhe mensagens, ou organize conversas pelo Bearer Token.</p></div><span className="docsQueueBadge">8 ROTAS</span></div>
          <div className="docsEndpoints docsGroupEndpoints">
            {messageActionEndpoints.map((item) => <button type="button" key={item.id} className={messageActionEndpoint === item.id ? 'active' : ''}
              onClick={() => { setMessageActionEndpoint(item.id); setMessageActionCopied(false) }}>
              <span className={`method-${item.method.toLowerCase()}`}>{item.method}</span><code>{item.path}</code><strong>{item.title}</strong>
            </button>)}
          </div>
          <div className="docsEndpointNote"><strong>{messageActionEndpointInfo.title}</strong><span>{messageActionEndpointInfo.description}</span></div>
          <div className="docsCodeHeader docsGroupCodeHeader"><div className="docsLanguages" role="tablist" aria-label="Linguagem do exemplo de mensagens e conversas">
            {languages.map((item) => <button key={item.id} type="button" role="tab" aria-selected={language === item.id}
              onClick={() => { setLanguage(item.id); setMessageActionCopied(false) }}>{item.label}</button>)}
          </div><button className="secondaryButton" type="button" onClick={() => void copyCode(messageActionCode, 'message-action')}>{messageActionCopied ? 'Copiado ✓' : 'Copiar código'}</button></div>
          <pre className="docsCode docsGroupCode" aria-label={`Exemplo de mensagens e conversas em ${languages.find((item) => item.id === language)?.label}`}><code>{messageActionCode}</code></pre>
          <p className="docsSecurity">Use telefone internacional sem <code>+</code> ou um JID de conversa. Para mensagens recebidas em grupos, informe também <code>participant</code>.</p>
        </section>

        <section className="docsSection" aria-labelledby="docs-organization-title">
          <div className="docsSectionTitle"><div><h3 id="docs-organization-title">Newsletters, etiquetas e comunidades</h3>
            <p>As opções finais de organização e canais, executadas diretamente pelo WhatsApp.</p></div><span className="docsQueueBadge">14 ROTAS</span></div>
          <div className="docsEndpoints docsGroupEndpoints">
            {organizationEndpoints.map((item) => <button type="button" key={item.id} className={organizationEndpoint === item.id ? 'active' : ''}
              onClick={() => { setOrganizationEndpoint(item.id); setOrganizationCopied(false) }}>
              <span className={`method-${item.method.toLowerCase()}`}>{item.method}</span><code>{item.path}</code><strong>{item.category} · {item.title}</strong>
            </button>)}
          </div>
          <div className="docsEndpointNote"><strong>{organizationEndpointInfo.title}</strong><span>{organizationEndpointInfo.description}</span></div>
          <div className="docsCodeHeader docsGroupCodeHeader"><div className="docsLanguages" role="tablist" aria-label="Linguagem do exemplo de newsletters, etiquetas e comunidades">
            {languages.map((item) => <button key={item.id} type="button" role="tab" aria-selected={language === item.id}
              onClick={() => { setLanguage(item.id); setOrganizationCopied(false) }}>{item.label}</button>)}
          </div><button className="secondaryButton" type="button" onClick={() => void copyCode(organizationCode, 'organization')}>{organizationCopied ? 'Copiado ✓' : 'Copiar código'}</button></div>
          <pre className="docsCode docsGroupCode" aria-label={`Exemplo de organização em ${languages.find((item) => item.id === language)?.label}`}><code>{organizationCode}</code></pre>
          <p className="docsSecurity">Em comunidades, os “participantes” desta operação são grupos existentes. Vincular ou desvincular não exclui o grupo.</p>
        </section>

        <section className="docsSection" aria-labelledby="docs-groups-title">
          <div className="docsSectionTitle"><div><h3 id="docs-groups-title">Gerenciamento de grupos</h3>
            <p>Liste, crie e administre grupos usando o Bearer Token da instância.</p></div><span className="docsQueueBadge">8 ROTAS</span></div>
          <div className="docsEndpoints docsGroupEndpoints">
            {groupEndpoints.map((item) => <button type="button" key={item.id} className={groupEndpoint === item.id ? 'active' : ''}
              onClick={() => { setGroupEndpoint(item.id); setGroupCopied(false) }}>
              <span className={`method-${item.method.toLowerCase()}`}>{item.method}</span><code>{item.path}</code><strong>{item.title}</strong>
            </button>)}
          </div>
          <div className="docsEndpointNote"><strong>{groupEndpointInfo.title}</strong><span>{groupEndpointInfo.description}</span></div>
          <div className="docsCodeHeader docsGroupCodeHeader"><div className="docsLanguages" role="tablist" aria-label="Linguagem do exemplo de grupos">
            {languages.map((item) => <button key={item.id} type="button" role="tab" aria-selected={language === item.id}
              onClick={() => { setLanguage(item.id); setCopied(false); setGroupCopied(false); setProfileCopied(false); setMediaCopied(false) }}>{item.label}</button>)}
          </div><button className="secondaryButton" type="button" onClick={() => void copyCode(groupCode, 'group')}>{groupCopied ? 'Copiado ✓' : 'Copiar código'}</button></div>
          <pre className="docsCode docsGroupCode" aria-label={`Exemplo de grupos em ${languages.find((item) => item.id === language)?.label}`}><code>{groupCode}</code></pre>
          <p className="docsSecurity">Substitua <code>120363012345678901@g.us</code> pelo JID retornado ao listar grupos. As permissões de administrador são validadas pelo WhatsApp.</p>
        </section>

        <section className="docsSection" aria-labelledby="docs-profile-title">
          <div className="docsSectionTitle"><div><h3 id="docs-profile-title">Perfil e privacidade</h3>
            <p>Consulte ou altere o perfil da conta conectada usando somente a API.</p></div><span className="docsQueueBadge">6 ROTAS</span></div>
          <div className="docsEndpoints docsProfileEndpoints">
            {profileEndpoints.map((item) => <button type="button" key={item.id} className={profileEndpoint === item.id ? 'active' : ''}
              onClick={() => { setProfileEndpoint(item.id); setProfileCopied(false) }}>
              <span className={`method-${item.method.toLowerCase()}`}>{item.method}</span><code>{item.path}</code><strong>{item.title}</strong>
            </button>)}
          </div>
          <div className="docsEndpointNote"><strong>{profileEndpointInfo.title}</strong><span>{profileEndpointInfo.description}</span></div>
          <div className="docsCodeHeader docsGroupCodeHeader"><div className="docsLanguages" role="tablist" aria-label="Linguagem do exemplo de perfil">
            {languages.map((item) => <button key={item.id} type="button" role="tab" aria-selected={language === item.id}
              onClick={() => { setLanguage(item.id); setCopied(false); setGroupCopied(false); setProfileCopied(false); setMediaCopied(false) }}>{item.label}</button>)}
          </div><button className="secondaryButton" type="button" onClick={() => void copyCode(profileCode, 'profile')}>{profileCopied ? 'Copiado ✓' : 'Copiar código'}</button></div>
          <pre className="docsCode docsGroupCode" aria-label={`Exemplo de perfil em ${languages.find((item) => item.id === language)?.label}`}><code>{profileCode}</code></pre>
          <div className="docsQueueFeatures docsPrivacyValues">
            <article><strong>Visibilidade comum</strong><span><code>all</code>, <code>contacts</code>, <code>contact_blacklist</code> ou <code>none</code>.</span></article>
            <article><strong>Confirmação de leitura</strong><span><code>readReceipts</code> aceita <code>all</code> ou <code>none</code>.</span></article>
            <article><strong>Online</strong><span><code>online</code> aceita <code>all</code> ou <code>match_last_seen</code>.</span></article>
            <article><strong>Demais opções</strong><span>Valores específicos inválidos retornam HTTP 422.</span></article>
          </div>
          <p className="docsSecurity">A foto deve ser JPEG de até 5 MB. Cada chamada de privacidade altera apenas o par <code>setting</code> e <code>value</code> informado.</p>
        </section>

        <section className="docsSection" aria-labelledby="docs-received-media-title">
          <div className="docsSectionTitle"><div><h3 id="docs-received-media-title">Mensagens e mídias recebidas</h3>
            <p>O webhook entrega os metadados completos e uma URL autenticada para baixar o arquivo descriptografado.</p></div><span className="docsQueueBadge">WEBHOOK</span></div>
          <div className="docsQueueFeatures">
            <article><strong>Tipos suportados</strong><span>Texto, imagem, vídeo, áudio, documento, figurinha, localização, contato e reação.</span></article>
            <article><strong>Armazenamento seguro</strong><span>Arquivos isolados por instância, privados e limitados a 100 MB.</span></article>
            <article><strong>Arquivo original</strong><span>Use <code>GET /api/messages/{'{messageID}'}/media</code>.</span></article>
            <article><strong>Resposta Base64</strong><span>Adicione <code>?format=base64</code> para receber JSON.</span></article>
          </div>
          <div className="docsEndpointNote"><strong>Evento message.received</strong><span>Quando <code>media.available</code> for verdadeiro, use <code>media.downloadUrl</code> com o Bearer Token da mesma instância.</span></div>
          <pre className="docsCode docsQueueCode" aria-label="Exemplo de webhook com mídia"><code>{receivedMediaEvent}</code></pre>
          <div className="docsCodeHeader docsGroupCodeHeader"><div className="docsLanguages" role="tablist" aria-label="Linguagem do download de mídia">
            {languages.map((item) => <button key={item.id} type="button" role="tab" aria-selected={language === item.id}
              onClick={() => { setLanguage(item.id); setMediaCopied(false) }}>{item.label}</button>)}
          </div><button className="secondaryButton" type="button" onClick={() => void copyCode(receivedCode, 'media')}>{mediaCopied ? 'Copiado ✓' : 'Copiar código'}</button></div>
          <pre className="docsCode docsGroupCode" aria-label={`Download de mídia em ${languages.find((item) => item.id === language)?.label}`}><code>{receivedCode}</code></pre>
          <p className="docsSecurity">O ID da mensagem não substitui a autenticação: cada mídia só pode ser acessada com o token da instância que a recebeu.</p>
        </section>

        <section className="docsSection" aria-labelledby="docs-webhook-title">
          <div className="docsSectionTitle"><div><h3 id="docs-webhook-title">Webhooks confiáveis</h3>
            <p>Teste a configuração e acompanhe cada entrega usando somente o Bearer Token da instância.</p></div><span className="docsQueueBadge">PERSISTENTE</span></div>
          <div className="docsQueueFeatures">
            <article><strong>Sobrevive a reinícios</strong><span>Eventos pendentes ficam armazenados no SQLite.</span></article>
            <article><strong>5 tentativas</strong><span>Reenvios após 1 min, 5 min, 15 min e 1 hora.</span></article>
            <article><strong>Retry-After</strong><span>O tempo solicitado pelo destino é respeitado, até 24 horas.</span></article>
            <article><strong>Idempotência</strong><span>Elimine duplicidades pelo header <code>X-Wirely-Delivery</code>.</span></article>
          </div>
          <div className="docsCodeHeader docsGroupCodeHeader"><div className="docsLanguages" role="tablist" aria-label="Linguagem do exemplo de webhook">
            {languages.map((item) => <button key={item.id} type="button" role="tab" aria-selected={language === item.id}
              onClick={() => { setLanguage(item.id); setWebhookCopied(false) }}>{item.label}</button>)}
          </div><button className="secondaryButton" type="button" onClick={() => void copyCode(webhookCode, 'webhook')}>{webhookCopied ? 'Copiado ✓' : 'Copiar código'}</button></div>
          <pre className="docsCode docsGroupCode" aria-label={`Exemplo de webhook em ${languages.find((item) => item.id === language)?.label}`}><code>{webhookCode}</code></pre>
          <p className="docsSecurity">Uma resposta HTTP 2xx conclui a entrega. O corpo é assinado em <code>X-Wirely-Signature</code>, e <code>X-Wirely-Attempt</code> informa a tentativa atual.</p>
        </section>

        <section className="docsSection" aria-labelledby="docs-queue-title">
          <div className="docsSectionTitle"><div><h3 id="docs-queue-title">Fila confiável</h3>
            <p>Envio persistente para integrações que não podem perder mensagens.</p></div><span className="docsQueueBadge">RECOMENDADO</span></div>
          <div className="docsQueueFeatures">
            <article><strong>Idempotência</strong><span>A mesma chave não cria mensagens duplicadas.</span></article>
            <article><strong>Agendamento</strong><span>Use <code>scheduledAt</code> em RFC 3339, até 365 dias.</span></article>
            <article><strong>Recuperação</strong><span>Até 5 tentativas com espera progressiva entre falhas.</span></article>
            <article><strong>Controle</strong><span>Consulte, cancele ou repita um envio pelo ID do job.</span></article>
          </div>
          <pre className="docsCode docsQueueCode" aria-label="Exemplo da fila confiável"><code>{queueCode}</code></pre>
          <p className="docsSecurity">Use <code>/api/queue/text</code> para texto e <code>/api/queue/media</code> para imagem, vídeo, áudio, documento ou figurinha. Mídias usam o mesmo multipart do envio síncrono.</p>
        </section>
      </div>
      <footer className="manageFooter"><span className="docsFooterLink">Limite de mídia: 32 MB · Fila: 5 tentativas</span>
        <button className="secondaryButton" type="button" onClick={onClose}>Fechar</button></footer>
    </dialog>
  )
}
