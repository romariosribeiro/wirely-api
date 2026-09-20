package server

import (
	"encoding/json"
	"net/http"
	"sync"
)

var extendedOpenAPI struct {
	sync.Once
	data []byte
}

func openAPI(w http.ResponseWriter, _ *http.Request) {
	extendedOpenAPI.Do(func() { extendedOpenAPI.data = buildExtendedOpenAPI() })
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "public, max-age=300")
	_, _ = w.Write(extendedOpenAPI.data)
}

func buildExtendedOpenAPI() []byte {
	var spec map[string]any
	if json.Unmarshal([]byte(openAPISpec), &spec) != nil {
		return []byte(openAPISpec)
	}
	spec["info"].(map[string]any)["version"] = "0.15.1"
	paths := spec["paths"].(map[string]any)
	spec["tags"] = append(spec["tags"].([]any), map[string]any{"name": "Status"})
	schemas := spec["components"].(map[string]any)["schemas"].(map[string]any)
	var additions map[string]any
	_ = json.Unmarshal([]byte(openAPIAdditionSchemas), &additions)
	for name, schema := range additions {
		schemas[name] = schema
	}

	addJSONPath(paths, "/api/send/location/live", "post", "Mensagens", "Enviar localização ao vivo (experimental)", "LiveLocationRequest", 201)
	addJSONPath(paths, "/api/messages/forward", "post", "Mensagens", "Encaminhar mensagem do histórico local", "ForwardMessageRequest", 201)
	addJSONPath(paths, "/api/chats/disappearing", "put", "Conversas", "Configurar mensagens temporárias", "DisappearingRequest", 200)
	paths["/api/presence"] = map[string]any{"post": bearerOperation("Conversas", "Definir presença global da instância", 200, "PresenceState", jsonBody("SetPresenceRequest"))}
	paths["/api/presence/subscribe"] = map[string]any{"post": bearerOperation("Conversas", "Assinar presença de um contato", 200, "PresenceState", jsonBody("PresencePhoneRequest"))}
	getPresence := bearerOperation("Conversas", "Consultar última presença recebida", 200, "PresenceState", nil)
	getPresence["parameters"] = []any{map[string]any{"name": "phone", "in": "path", "required": true, "schema": map[string]any{"type": "string"}}}
	paths["/api/presence/{phone}"] = map[string]any{"get": getPresence}
	paths["/api/events"] = map[string]any{"get": map[string]any{"tags": []string{"Atividade"}, "summary": "Receber eventos em tempo real por SSE", "security": []any{map[string]any{"BearerAuth": []any{}}}, "parameters": []any{map[string]any{"name": "events", "in": "query", "schema": map[string]any{"type": "string"}, "description": "Eventos separados por vírgula"}}, "responses": map[string]any{"200": map[string]any{"description": "Fluxo Server-Sent Events", "content": map[string]any{"text/event-stream": map[string]any{"schema": map[string]any{"type": "string"}}}}}}}
	addJSONPath(paths, "/api/status/text", "post", "Status", "Publicar Status de texto", "StatusTextRequest", 201)
	paths["/api/status/media"] = map[string]any{"post": bearerOperation("Status", "Publicar Status com imagem ou vídeo", 201, "SentMessage", map[string]any{"required": true, "content": map[string]any{"multipart/form-data": map[string]any{"schema": map[string]any{"$ref": "#/components/schemas/StatusMediaRequest"}}}})}
	addJSONPath(paths, "/api/groups/{groupJID}/description", "patch", "Grupos", "Alterar descrição do grupo", "GroupDescriptionRequest", 204)
	paths["/api/groups/{groupJID}/photo"] = map[string]any{
		"put":    bearerOperation("Grupos", "Definir foto do grupo", 200, "PictureResult", map[string]any{"required": true, "content": map[string]any{"multipart/form-data": map[string]any{"schema": map[string]any{"$ref": "#/components/schemas/PhotoUpload"}}}}),
		"delete": bearerOperation("Grupos", "Remover foto do grupo", 204, "", nil),
	}
	paths["/api/groups/{groupJID}/leave"] = map[string]any{"post": bearerOperation("Grupos", "Sair do grupo", 204, "", nil)}
	addJSONPath(paths, "/api/groups/{groupJID}/permissions", "patch", "Grupos", "Controlar envio e edição de informações", "GroupPermissionsRequest", 204)
	addJSONPath(paths, "/api/groups/{groupJID}/join-approval", "put", "Grupos", "Ativar aprovação de novos membros", "GroupJoinApprovalRequest", 204)
	paths["/api/groups/{groupJID}/join-requests"] = map[string]any{
		"get":  bearerOperation("Grupos", "Listar solicitações de entrada", 200, "GroupJoinRequestList", nil),
		"post": bearerOperation("Grupos", "Aprovar ou rejeitar solicitações", 200, "GroupParticipantList", jsonBody("GroupJoinRequestAction")),
	}
	for _, path := range []string{
		"/api/groups/{groupJID}/description",
		"/api/groups/{groupJID}/photo",
		"/api/groups/{groupJID}/leave",
		"/api/groups/{groupJID}/permissions",
		"/api/groups/{groupJID}/join-approval",
		"/api/groups/{groupJID}/join-requests",
	} {
		for _, operation := range paths[path].(map[string]any) {
			operation.(map[string]any)["parameters"] = []any{map[string]any{
				"name": "groupJID", "in": "path", "required": true,
				"description": "JID completo do grupo retornado por GET /api/groups.",
				"schema":      map[string]any{"type": "string", "pattern": "@g\\.us$"},
			}}
		}
	}

	addAdvancedMessageProperties(schemas, "TextRequest", true)
	addAdvancedMessageProperties(schemas, "LocationRequest", false)
	addAdvancedMessageProperties(schemas, "ContactRequest", false)
	media := spec["components"].(map[string]any)["requestBodies"].(map[string]any)["Media"].(map[string]any)
	mediaProps := media["content"].(map[string]any)["multipart/form-data"].(map[string]any)["schema"].(map[string]any)["properties"].(map[string]any)
	mediaProps["messageOptions"] = map[string]any{"type": "string", "description": "JSON com replyTo, mentions e forwarded"}
	mediaProps["viewOnce"] = map[string]any{"type": "boolean", "description": "Somente imagem ou vídeo"}

	data, err := json.MarshalIndent(spec, "", "  ")
	if err != nil {
		return []byte(openAPISpec)
	}
	return data
}

func addAdvancedMessageProperties(schemas map[string]any, name string, linkPreview bool) {
	properties := schemas[name].(map[string]any)["properties"].(map[string]any)
	properties["replyTo"] = map[string]any{"$ref": "#/components/schemas/ReplyOptions", "description": "Mensagem que será respondida."}
	properties["mentions"] = map[string]any{"type": "array", "maxItems": 100, "description": "Telefones ou JIDs mencionados na mensagem.", "items": map[string]any{"type": "string"}}
	properties["forwarded"] = map[string]any{"type": "boolean", "description": "Exibe a indicação de mensagem encaminhada."}
	if linkPreview {
		properties["linkPreview"] = map[string]any{"type": "boolean", "description": "Gera automaticamente a prévia do primeiro link."}
	}
}

func addJSONPath(paths map[string]any, path, method, tag, summary, requestSchema string, status int) {
	paths[path] = map[string]any{method: bearerOperation(tag, summary, status, responseSchema(status), jsonBody(requestSchema))}
}

func responseSchema(status int) string {
	if status == 201 {
		return "SentMessage"
	}
	if status == 204 {
		return ""
	}
	return "ChatActionResult"
}
func jsonBody(schema string) map[string]any {
	return map[string]any{"required": true, "content": map[string]any{"application/json": map[string]any{"schema": map[string]any{"$ref": "#/components/schemas/" + schema}}}}
}
func bearerOperation(tag, summary string, status int, schema string, body map[string]any) map[string]any {
	response := map[string]any{"description": "Operação concluída"}
	if schema != "" {
		response["content"] = map[string]any{"application/json": map[string]any{"schema": map[string]any{"$ref": "#/components/schemas/" + schema}}}
	}
	operation := map[string]any{"tags": []string{tag}, "summary": summary, "security": []any{map[string]any{"BearerAuth": []any{}}}, "responses": map[string]any{string(rune('0'+status/100)) + string(rune('0'+status/10%10)) + string(rune('0'+status%10)): response}}
	if body != nil {
		operation["requestBody"] = body
	}
	return operation
}

const openAPIAdditionSchemas = `{
  "ReplyOptions":{"type":"object","required":["messageId"],"properties":{"messageId":{"type":"string","description":"ID da mensagem respondida."},"participant":{"type":"string","description":"Autor da mensagem; necessário ao responder mensagens recebidas em grupo."},"text":{"type":"string","description":"Texto citado opcional quando disponível."}}},
  "LiveLocationRequest":{"type":"object","required":["recipient","latitude","longitude"],"properties":{"recipient":{"type":"string"},"latitude":{"type":"number","minimum":-90,"maximum":90},"longitude":{"type":"number","minimum":-180,"maximum":180},"accuracy":{"type":"integer"},"speed":{"type":"number"},"bearing":{"type":"integer","maximum":359},"caption":{"type":"string"},"sequence":{"type":"integer"},"timeOffset":{"type":"integer"}}},
  "ForwardMessageRequest":{"type":"object","required":["chat","messageId","recipient"],"properties":{"chat":{"type":"string"},"messageId":{"type":"string"},"recipient":{"type":"string"}}},
  "DisappearingRequest":{"type":"object","required":["chat","duration"],"properties":{"chat":{"type":"string"},"duration":{"type":"string","enum":["off","24h","7d","90d"]}}},
  "SetPresenceRequest":{"type":"object","required":["presence"],"properties":{"presence":{"type":"string","enum":["available","unavailable"]}}},
  "PresencePhoneRequest":{"type":"object","required":["phone"],"properties":{"phone":{"type":"string"}}},
  "PresenceState":{"type":"object","required":["presence"],"properties":{"phone":{"type":"string"},"jid":{"type":"string"},"presence":{"type":"string","enum":["available","unavailable","unknown"]},"lastSeen":{"type":"string","format":"date-time"},"updatedAt":{"type":"string","format":"date-time"}}},
  "StatusTextRequest":{"type":"object","required":["text"],"properties":{"text":{"type":"string","maxLength":700},"background":{"type":"integer"},"textColor":{"type":"integer"},"font":{"type":"integer","minimum":0,"maximum":10}}},
  "StatusMediaRequest":{"type":"object","required":["type","file"],"properties":{"type":{"type":"string","enum":["image","video"]},"file":{"type":"string","format":"binary"},"caption":{"type":"string"}}},
  "GroupDescriptionRequest":{"type":"object","required":["description"],"properties":{"description":{"type":"string","maxLength":2048}}},
  "PhotoUpload":{"type":"object","required":["file"],"properties":{"file":{"type":"string","format":"binary"}}},
  "PictureResult":{"type":"object","properties":{"pictureId":{"type":"string"}}},
  "GroupPermissionsRequest":{"type":"object","properties":{"announce":{"type":"boolean","description":"Somente administradores enviam"},"locked":{"type":"boolean","description":"Somente administradores editam informações"}}},
  "GroupJoinApprovalRequest":{"type":"object","required":["enabled"],"properties":{"enabled":{"type":"boolean"}}},
  "GroupJoinRequestAction":{"type":"object","required":["action","participants"],"properties":{"action":{"type":"string","enum":["approve","reject"]},"participants":{"type":"array","items":{"type":"string"}}}},
  "GroupJoinRequestList":{"type":"object","properties":{"data":{"type":"array","items":{"type":"object","properties":{"jid":{"type":"string"},"phone":{"type":"string"},"requestedAt":{"type":"string","format":"date-time"}}}}}},
  "GroupParticipantList":{"type":"object","properties":{"data":{"type":"array","items":{"$ref":"#/components/schemas/GroupParticipant"}}}}
}`
