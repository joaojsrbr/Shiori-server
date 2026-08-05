# Shiori Backend

Área reservada ao servidor Go e aos workers auxiliares.
O Shiori Backend é uma API robusta projetada para extrair dados estruturados (mangás, animes) de páginas da web, utilizando automação de navegadores (ChromeDP) em conjunto com Inteligência Artificial Local (LM Studio / Llama).

## Arquitetura e Componentes

O backend segue um modelo de API HTTP + Background Workers para tarefas longas.

* **API HTTP (`cmd/api` & `internal/platform/httpserver`):**
    Expõe os endpoints principais e gerencia rotas.
* **Fila e Workers (`internal/platform/queue` & `internal/worker`):**
    A extração de mídia via IA leva tempo (minutos). O sistema de fila permite enfileirar jobs (`/api/v1/jobs/extract`) sem bloquear o cliente. O worker processa essa fila em background de forma assíncrona.
* **Barramento de Eventos (`internal/platform/events/hub.go`):**
    Sistema de Pub/Sub em memória. Permite que o Frontend se conecte via **Server-Sent Events (SSE)** em `GET /api/v1/jobs/{id}/events` e acompanhe em tempo real os logs de navegação, captura e extração de IA do worker que está processando o job.
* **Extração e IA (`internal/extraction` & `internal/platform/ai/lmstudio`):**
    Responsável por formatar os *prompts* e se comunicar com a LLM para converter HTML destilado (Markdown) em JSON estruturado.
* **Browser e Desafios (`internal/platform/browser`):**
    Utiliza ChromeDP para navegar nas páginas. Quando Cloudflare, CAPTCHA ou login exigem interação, a aplicação emite um evento `challenge` no SSE e oferece um handoff temporário para o usuário concluir a ação manualmente.

## Endpoints Principais

| Método | Rota | Descrição |
| :--- | :--- | :--- |
| `POST` | `/api/v1/jobs/extract` | Enfileira uma URL para ser extraída em background. Retorna um `job_id`. |
| `GET` | `/api/v1/jobs/{job_id}/events` | SSE: Conexão persistente para receber logs em tempo real sobre o progresso do job. |
| `POST` | `/api/v1/debug/extract` | Rota SSE síncrona, registrada exclusivamente com `--log-level debug`. |
| `GET` | `/api/v1/capabilities` | Retorna o status de conexão dos drivers (Banco de Dados, LMStudio, Navegador, Fila). |

## Como Executar

### Pré-requisitos

1. Go 1.22+
2. LM Studio rodando localmente, com `SHIORI_AI_MAX_CONTEXT_LENGTH` igual à janela realmente carregada (padrão: `8192`).
3. Google Chrome instalado no sistema (para o ChromeDP).

No perfil portátil Windows, o servidor baixa o FlareSolverr 3.5.0 para
`.shiori/FlareSolverr/` quando `flaresolverr.exe` ainda não existe e inicia o
processo oculto em `127.0.0.1:8191` junto com o Shiori.

O caminho configurado por `SHIORI_DATA_DIR` representa a própria pasta
privada do Shiori. Com `SHIORI_DATA_DIR=./.shiori`, a estrutura é:

```text
.shiori/shiori.db
.shiori/FlareSolverr/
.shiori/browser-profiles/
.shiori/storage/
```

No Docker, o serviço já está incluído no `compose.yaml`. Para executá-lo
separadamente:

```powershell
docker run -d --name=flaresolverr -p 127.0.0.1:8191:8191 -e LOG_LEVEL=info --restart unless-stopped ghcr.io/flaresolverr/flaresolverr:latest
```

### Scripts Bash

No Linux, macOS, WSL ou Git Bash:

```bash
./scripts/build.sh
./scripts/smoke-test.sh
./scripts/test-api.sh
./scripts/test-lycantoons.sh
```

O `build.sh` gera o executavel Windows `dist/shiori-server.exe` sem executar
testes automaticamente.

### Passos

1. Entre na pasta `back`:

   ```bash
   cd back
   ```

2. Instale as dependências:

   ```bash
   go mod tidy
   ```

3. Rode a aplicação (as variáveis de ambiente ficam na pasta `config/.env`):

   ```bash
   go run ./cmd/api
   ```

A API subirá em `http://localhost:8080`.
