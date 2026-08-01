# ADR-006: Automação de Navegador

**Status:** Aceito  
**Data:** 2026-07-30

## Contexto

Algumas fontes de conteúdo requerem JavaScript para renderizar páginas, e
outras estão protegidas por Cloudflare ou outros desafios interativos. O
backend precisa de um navegador automatizado para essas situações, mas:

- o executável portátil não pode incluir Node.js ou Playwright;
- o Docker pode usar Playwright em um container separado;
- o sistema **nunca** resolve CAPTCHAs automaticamente;
- desafios interativos requerem handoff ao usuário.

## Decisão

Implementar uma interface `BrowserProvider` com dois adapters.

### 1. chromedp (perfil `portable`)

- Biblioteca Go pura via Chrome DevTools Protocol.
- Compila com `CGO_ENABLED=0`.
- Localiza Chrome ou Edge por configuração e caminhos conhecidos do Windows.
- Perfil persistente isolado em `data/browser-profiles/<domínio>/`.
- Nunca reutiliza o perfil pessoal do usuário.
- Navega em modo headless por padrão.
- Abre janela **visível** quando o estado do job for `requires_user_action`
  (challenge, CAPTCHA, login).
- O usuário conclui o desafio manualmente.
- Após confirmação (via API), o backend retoma o job.
- Se Chrome/Edge não estiver disponível, a capability de navegador é desativada
  — o servidor continua funcionando.

### 2. Worker Playwright (perfil `docker`)

- Container separado com Node.js + Playwright em TypeScript.
- Código em `back/workers/browser/`.
- Comunicação com o servidor Go via HTTP interno (REST simples).
- O worker recebe comandos restritos:
  - `navigate(url, waitFor, timeout)`
  - `snapshot()` → DOM sanitizado + URL final + status + headers
  - `listAssets()` → lista de URLs de assets observados
  - `getSessionState()` → estado para `requires_user_action`
- O worker **não** contém regras de negócio.
- O worker **não** acessa o banco diretamente.
- O Go valida e sanitiza tudo que receber do worker.
- Base de imagem oficial compatível com a versão do Playwright.

### Interface comum

```go
type BrowserProvider interface {
    Navigate(ctx context.Context, req NavigateRequest) (*NavigateResult, error)
    Snapshot(ctx context.Context, sessionID string) (*PageSnapshot, error)
    CloseSession(ctx context.Context, sessionID string) error
    IsAvailable() bool
}
```
