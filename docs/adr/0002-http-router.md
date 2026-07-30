# ADR-002: Router HTTP

**Status:** Aceito  
**Data:** 2026-07-30

## Contexto

O backend precisa de um multiplexador HTTP que suporte:

- grupos de rotas com prefixo (`/api/v1`, `/health`);
- cadeia de middlewares composáveis (logging, recovery, CORS, request ID,
  autenticação);
- path parameters (`/media/{mediaId}`);
- compatibilidade total com `net/http`;
- SSE e WebSocket sem abstrações que bloqueiem streaming.

O Go 1.22+ inclui um `ServeMux` com suporte a métodos e path params, mas não
oferece agrupamento de rotas nem composição de middlewares nativamente.

## Decisão

Usar **chi v5** (`github.com/go-chi/chi/v5`) como router com **handlers
manuais**.

Não será usado gerador de código (como ogen) para handlers. A validação de
request/response será feita com helpers customizados, mantendo controle total
sobre o código dos handlers.

### Razões

- chi é 100% compatível com `http.Handler` e `http.HandlerFunc`.
- Middlewares são `func(http.Handler) http.Handler` — padrão Go idiomático.
- `chi.Router` oferece `Group`, `Route`, `With`, `Mount` para composição.
- Sem reflexão, sem geração de código, sem lock-in.
- Maduro e amplamente adotado (v5.3.1, julho 2026).

### Alternativas descartadas

| Opção | Motivo da rejeição |
|---|---|
| `net/http.ServeMux` puro | Sem grupos, sem middleware chain, verboso |
| ogen (spec-first) | Gera handlers acoplados ao gerador; perde controle |
| Gin / Echo | API opinada, `gin.Context` / `echo.Context` divergem de `net/http` |
| gorilla/mux | Maintenance mode desde 2022 |

## Consequências

- Handlers são funções `func(w http.ResponseWriter, r *http.Request)`.
- Validação de request body, query params e path params será feita por helpers
  do projeto (ex: `httputil.DecodeJSON`, `httputil.PathParam`).
- Middlewares de logging, recovery, request ID e CORS serão configurados no
  router raiz.
- O OpenAPI YAML será a referência do contrato, mas não gerará código Go.
- O time é responsável por manter handlers e spec sincronizados manualmente.
  Testes de contrato mitigarão drift.
