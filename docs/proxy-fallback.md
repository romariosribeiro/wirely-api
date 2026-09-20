# Reconexão com saída direta

Quando uma tentativa de conexão pelo proxy configurado falha, o motor passa a
usar conexão direta pela VPS. Isso cobre falhas no início da sessão e falhas
nas tentativas de reconexão do Whatsmeow. A detecção depende dos timeouts de
rede e de keepalive; a troca não é instantânea ao desligar o túnel.

O proxy salvo no SQLite não é apagado. A saída direta permanece ativa nessa
sessão, inclusive nas próximas reconexões. Reiniciar o serviço ou reaplicar o
proxy volta a tentar o endereço configurado. Não há retorno automático ao proxy
enquanto a conexão direta estiver funcionando.

O fallback vale para instâncias com proxy configurado. Logout, substituição da
sessão e desconexão manual não devem provocar reconexão pela saída direta.

`GET /api/instance/status` e `GET /api/instances/{id}/state` incluem:

- `connectionRoute`: `proxy` ou `direct`, a rota selecionada para conexão.
- `proxyFallback`: indica que a rota direta foi selecionada após falha do proxy.

Esses campos não substituem `status`: a rota direta também pode estar sem
conectividade. O painel Gerenciar consulta esse estado a cada cinco segundos.

O journal registra falhas de reconexão, mudança para saída direta e mudanças de
estado com o ID da instância, sem registrar URL/credenciais do proxy.

Validação:

```sh
go test ./internal/engine ./internal/server
go test -race ./internal/engine
cd web
npm run lint
npm run build
```
