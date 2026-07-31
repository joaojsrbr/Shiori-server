# Shiori Backend

Área reservada ao servidor Go e aos workers auxiliares. 
O Shiori Backend é uma API robusta projetada para extrair dados estruturados (mangás, animes) de páginas da web, utilizando automação de navegadores (ChromeDP) em conjunto com Inteligência Artificial Local (LM Studio / Llama).

Antes de implementar, leia [docs/prompts/PROMPT.md](docs/prompts/PROMPT.md).
A memória persistente do projeto fica em [docs/megabrain/README.md](docs/megabrain/README.md).

## Arquitetura e Componentes

O backend segue um modelo de API HTTP + Background Workers para tarefas longas.

*   **API HTTP (`cmd/api` & `internal/platform/httpserver`):** 
    Expõe os endpoints principais e gerencia rotas.
*   **Fila e Workers (`internal/platform/queue` & `internal/worker`):**
    A extração de mídia via IA leva tempo (minutos). O sistema de fila permite enfileirar jobs (`/api/v1/jobs/extract`) sem bloquear o cliente. O worker processa essa fila em background de forma assíncrona.
*   **Barramento de Eventos (`internal/platform/events/hub.go`):**
    Sistema de Pub/Sub em memória. Permite que o Frontend se conecte via **Server-Sent Events (SSE)** em `GET /api/v1/jobs/{id}/events` e acompanhe em tempo real os logs de navegação, captura e extração de IA do worker que está processando o job.
*   **Extração e IA (`internal/extraction` & `internal/platform/ai/lmstudio`):**
    Responsável por formatar os *prompts* e se comunicar com a LLM para converter HTML destilado (Markdown) em JSON estruturado.
*   **Browser e Desafios (`internal/platform/browser`):**
    Utiliza ChromeDP para navegar nas páginas e contornar desafios (ex: Cloudflare Turnstile). Caso seja detectado um CAPTCHA irresolvível automaticamente, a aplicação emite um evento "challenge" no SSE e expõe um WebSocket Proxy temporário (`/api/v1/challenges/{token}`) para o usuário resolver o desafio manualmente através da UI.

## Endpoints Principais

| Método | Rota | Descrição |
| :--- | :--- | :--- |
| `POST` | `/api/v1/jobs/extract` | Enfileira uma URL para ser extraída em background. Retorna um `job_id`. |
| `GET`  | `/api/v1/jobs/{job_id}/events` | SSE: Conexão persistente para receber logs em tempo real sobre o progresso do job. |
| `POST` | `/api/v1/debug/extract` | Rota para ambiente de desenvolvimento. Funciona igual ao fluxo do job, porém o request fica preso (bloqueante) disparando SSE direto na resposta. Retorna a extração imediatamente ao finalizar. |
| `GET`  | `/api/v1/capabilities` | Retorna o status de conexão dos drivers (Banco de Dados, LMStudio, Navegador, Fila). |

## Como Executar

### Pré-requisitos
1. Go 1.22+
2. LM Studio rodando localmente (preferencialmente um modelo com janela de contexto maior, ex: `65536`).
3. Google Chrome instalado no sistema (para o ChromeDP).

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
