# ADR-007: Contrato e Geração OpenAPI

**Status:** Aceito  
**Data:** 2026-07-30

## Contexto

O contrato OpenAPI é a fonte de verdade para a integração entre o backend Go
e o frontend Flutter. As opções são:

1. **Spec-first com gerador (ogen):** escrever YAML, gerar handlers Go
   automaticamente.
2. **Spec-first com handlers manuais:** escrever YAML, implementar handlers
   no chi manualmente, validar com testes de contrato.
3. **Code-first:** gerar spec a partir de anotações no código Go.

## Decisão

Usar abordagem **spec-first com handlers manuais no chi**.

### Fluxo

1. O contrato é escrito manualmente em
   `back/api/openapi/shiori.yaml` (OpenAPI 3.1).
2. Handlers são implementados manualmente como `http.HandlerFunc` registrados
   no chi router.
3. Validação de request/response é feita por helpers customizados do projeto.
4. Testes de contrato verificam que handlers e spec estão sincronizados.
5. O frontend Flutter gera o cliente Dart a partir do mesmo YAML.
6. O YAML é embutido no executável via `go:embed` e servido em
   `GET /api/v1/openapi.yaml`.

### Razões para handlers manuais

- Controle total sobre lógica de request/response.
- Sem dependência de gerador de código no build.
- Sem código gerado para manter/revisar.
- Handlers são `http.HandlerFunc` puros — testáveis sem framework.
- O custo de manter sincronia entre spec e handlers é mitigado por testes de
  contrato.

### Helpers de validação

O projeto terá um pacote `internal/platform/httpserver/` com:

```go
// Decodificação JSON com limite de body
func DecodeJSON(r *http.Request, v any) error

// Resposta JSON padronizada
func RespondJSON(w http.ResponseWriter, status int, v any)

// Erro RFC 9457 (application/problem+json)
func RespondError(w http.ResponseWriter, problem Problem)

// Path params via chi
func PathParam(r *http.Request, key string) string

// Query params com parsing e validação
func QueryParam(r *http.Request, key string) string
```

### Convenções do contrato

- Base path: `/api/v1`
- IDs: UUID ou ULID opaco como string
- Timestamps: ISO 8601 UTC
- Paginação: cursor-based
- Erros: `application/problem+json` (RFC 9457)
- Idempotência: header `Idempotency-Key`
- Correlação: header `X-Request-ID`
- Concorrência: `ETag` / `If-Match` quando necessário

### Alternativas descartadas

| Opção | Motivo da rejeição |
|---|---|
| ogen (spec-first + gerador) | Gera código acoplado ao gerador; perde controle sobre handlers |
| swaggo/swag (code-first) | Spec derivada de comentários; dificulta spec estável para Dart |
| oapi-codegen | Semelhante a ogen; geração cria camada de indireção desnecessária |

## Consequências

- O time é responsável por manter spec e handlers sincronizados.
- Testes de contrato (ex: validar rotas registradas vs paths da spec) são
  obrigatórios para detectar drift.
- O frontend pode gerar o cliente Dart a qualquer momento a partir do YAML.
- Mudanças no contrato devem atualizar spec, handlers e testes ao mesmo tempo.
- O YAML servido em runtime permite que ferramentas externas (Swagger UI,
  Postman) consumam a API.
