# Shiori Backend

Servidor principal e workers auxiliares do Shiori em Go.

O **Shiori Backend** é uma API robusta e executável portátil projetado para indexar, organizar, arquivar e sincronizar mídias (mangás, manhwas, quadrinhos, novels e animes), coordenando downloads e utilizando automação de navegador real e Inteligência Artificial Local (LM Studio com NuExtract).

---

## Arquitetura e Componentes

O servidor opera em arquitetura limpa com suporte a dois perfis de infraestrutura (`portable` e `docker`):

* **API HTTP (`cmd/api` & `internal/platform/httpserver`):**
  Servidor web em Go utilizando Chi Router para gerenciar rotas versionadas (`/api/v1`) e rotas de saúde (`/health/live`, `/health/ready`).
* **Fila Durável e Workers (`internal/platform/queue` & `internal/worker`):**
  Fila durável local em SQLite no perfil portátil (ou Valkey/Redis no perfil Docker) para processamento assíncrono idempotente com retry e controle de concorrência.
* **Barramento de Eventos (`internal/platform/events/hub.go`):**
  Sistema Pub/Sub para streaming via **Server-Sent Events (SSE)** em `GET /api/v1/jobs/{id}/events`, acompanhando em tempo real os logs de navegação, captura e extração de IA.
* **Navegador e Desafios (`internal/platform/browser` & `internal/jobs/challenge_handler.go`):**
  Integração com navegador real (ChromeDP + FlareSolverr) para contornar proteções Cloudflare e realizar handoff interativo via WebSocket quando necessário.
* **IA e NuExtract (`internal/extraction` & `internal/platform/ai/lmstudio`):**
  Integração com o LM Studio local para extração estruturada de metadados em modelos NuExtract Tiny, Default e Quality.

---

## Endpoints da API

Todas as rotas definidas na especificação OpenAPI `api/openapi/shiori.yaml` estão implementadas e ativas:

| Categoria | Método | Rota | Descrição |
| :--- | :--- | :--- | :--- |
| **Health** | `GET` | `/health/live` | Probe de liveness |
| **Health** | `GET` | `/health/ready` | Probe de readiness |
| **Sistema** | `GET` | `/api/v1/capabilities` | Retorna status dos drivers (banco, fila, storage, browser, IA) |
| **Sistema** | `GET` | `/api/v1/profiles` | Retorna lista de perfis de leitura locais |
| **Sistema** | `GET` | `/api/v1/settings` | Obtém as configurações do usuário |
| **Sistema** | `PUT` | `/api/v1/settings` | Atualiza as configurações do usuário |
| **Biblioteca** | `GET` | `/api/v1/media` | Listagem paginada e filtrada de mídias |
| **Biblioteca** | `POST` | `/api/v1/media` | Adiciona uma nova mídia à biblioteca |
| **Biblioteca** | `GET` | `/api/v1/media/{mediaId}` | Obtém detalhes de uma mídia por ID |
| **Biblioteca** | `DELETE` | `/api/v1/media/{mediaId}` | Remove a mídia, capítulos, histórico e assets |
| **Biblioteca** | `GET` | `/api/v1/media/{mediaId}/chapters` | Lista capítulos ou episódios |
| **Biblioteca** | `GET` | `/api/v1/chapters/{chapterId}` | Detalhes de um capítulo para leitor/player |
| **Biblioteca** | `GET` | `/api/v1/reader/assets/{assetPath}` | Servidor de páginas/imagens salvas em storage local |
| **IA / LLM** | `GET` | `/api/v1/ai/models` | Lista modelos configurados no LM Studio |
| **IA / LLM** | `POST` | `/api/v1/ai/models/{modelKey}/load` | Carrega modelo específico na memória do LM Studio |
| **IA / LLM** | `POST` | `/api/v1/ai/models/{modelKey}/unload` | Descarrega modelo da memória |
| **Jobs** | `POST` | `/api/v1/jobs/extract` | Enfileira job de extração assíncrona por URL |
| **Jobs** | `GET` | `/api/v1/jobs/{jobID}` | Obtém o status durável de um job |
| **Jobs** | `DELETE` | `/api/v1/jobs/{jobID}` | Cancela cooperativamente um job |
| **Jobs** | `GET` | `/api/v1/jobs/{jobID}/events` | Stream SSE de progresso e eventos do job |
| **Coleções** | `GET` | `/api/v1/collections` | Lista coleções criadas |
| **Coleções** | `POST` | `/api/v1/collections` | Cria uma nova coleção |
| **Coleções** | `GET` | `/api/v1/collections/{collectionId}` | Obtém detalhes de uma coleção |
| **Coleções** | `PATCH` | `/api/v1/collections/{collectionId}` | Renomeia ou altera descrição |
| **Coleções** | `DELETE` | `/api/v1/collections/{collectionId}` | Exclui uma coleção |
| **Coleções** | `GET` | `/api/v1/collections/{collectionId}/media` | Lista mídias pertencentes à coleção |
| **Coleções** | `PUT` | `/api/v1/collections/{collectionId}/media/{mediaId}` | Adiciona mídia à coleção |
| **Coleções** | `DELETE` | `/api/v1/collections/{collectionId}/media/{mediaId}` | Remove mídia da coleção |
| **Histórico** | `GET` | `/api/v1/history` | Lista histórico de leitura |
| **Histórico** | `PUT` | `/api/v1/history/{chapterId}` | Salva progresso de leitura |
| **Histórico** | `DELETE` | `/api/v1/history/{chapterId}` | Remove entrada do histórico |
| **Downloads** | `GET` | `/api/v1/downloads` | Lista capítulos baixados em storage local |
| **Downloads** | `DELETE` | `/api/v1/downloads/{chapterId}` | Remove imagens salvas de um capítulo |
| **Filtros** | `GET` | `/api/v1/filters/presets` | Lista presets de filtros salvos |
| **Filtros** | `POST` | `/api/v1/filters/presets` | Cria um novo preset de filtro |
| **Navegador** | `GET` | `/api/v1/browser/history` | Lista histórico de páginas navegadas no browser interno |
| **Desafio** | `GET` | `/api/v1/challenges/{token}` | Interface HTML para interação com navegador remoto |
| **Desafio** | `GET` | `/api/v1/challenges/{token}/status` | Status e tempo restante da sessão de desafio |
| **Desafio** | `POST` | `/api/v1/challenges/{token}/complete` | Solicita verificação de conclusão do desafio |
| **Desafio** | `DELETE` | `/api/v1/challenges/{token}` | Cancela sessão de desafio humano |
| **Desafio** | `GET` | `/api/v1/challenges/{token}/ws` | WebSocket para streaming de tela e envio de comandos |

---

## Como Executar

### Pré-requisitos

1. Go 1.22+
2. LM Studio rodando localmente (padrão: `http://127.0.0.1:1234`)
3. Google Chrome instalado no sistema (para o ChromeDP)

### Perfil Portátil (Windows)

O artefato portátil oficial é um único arquivo `shiori-server.exe` que cria e gerencia suas pastas de dados (`data/`, `storage/`, `logs/`) automaticamente ao seu lado.

No perfil portátil, o servidor baixa o FlareSolverr automaticamente para `data/FlareSolverr/` quando `flaresolverr.exe` ainda não existe.

```powershell
# Executar diretamente via Go
go run ./cmd/api --profile portable

# Ou compilar e executar o binário:
bash scripts/build.sh
.\dist\shiori-server.exe serve --profile portable
```

### Perfil Docker

Utilize o Docker Compose para subir a infraestrutura completa (PostgreSQL, Valkey/Redis, MinIO, Playwright e API):

```powershell
docker compose up -d --build
```

---

## Automação de Release (GitHub Actions)

O repositório possui um workflow em `.github/workflows/release.yml` que compila o executável Windows (`shiori-server.exe`) e cria uma Release no GitHub automaticamente ao receber uma tag `v*`.

### Gerando Tags Automaticamente

**PowerShell:**
```powershell
# Cria e incrementa a próxima tag (ex: v1.0.0 -> v1.0.1) e envia para o GitHub:
.\scripts\tag-release.ps1 -Push
```

**Bash / Linux / WSL:**
```bash
./scripts/tag-release.sh --push
```

---

## Scripts Disponíveis

| Script | Descrição |
| :--- | :--- |
| `scripts/build.sh` | Compila `dist/shiori-server.exe` (Windows portátil) |
| `scripts/smoke-test.sh` | Smoke test do executável em pasta vazia |
| `scripts/test-api.sh` | Testes de integração da API |
| `scripts/tag-release.ps1` | Incrementa e cria tag Git automaticamente (PowerShell) |
| `scripts/tag-release.sh` | Incrementa e cria tag Git automaticamente (Bash) |

---

## Testes e Validação

```bash
gofmt -w .
go vet ./...
go test ./...
```
