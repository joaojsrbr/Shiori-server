# ADR-008: Handoff humano para desafios do navegador

**Status:** Aceito
**Data:** 2026-08-01

## Contexto

Algumas fontes autorizadas apresentam challenge, CAPTCHA ou login que não pode
ser resolvido automaticamente. Retornar apenas `409` perde a sessão do
navegador; abrir o perfil pessoal do usuário mistura credenciais; aceitar um
WebSocket irrestrito expõe a sessão a controle cruzado.

## Decisão

O backend mantém a sessão isolada e entrega ao frontend uma URL relativa,
temporária e descartável em `/api/v1/challenges/{token}`. Essa página:

- transmite frames JPEG do navegador por WebSocket usando CDP Screencast;
- sincroniza o viewport remoto com toda a área útil da página de handoff e
  recalcula as coordenadas de interação após redimensionamentos;
- encaminha somente eventos permitidos de ponteiro, roda do mouse e teclado;
- nunca expõe cookies, DevTools, session ID ou HTML interno ao cliente;
- aceita um único controlador de mesma origem por token;
- expira em três minutos e oferece cancelamento explícito;
- pede ao backend para verificar novamente o DOM antes de concluir;
- só retoma o job quando o challenge não estiver mais visível.
- classifica `challenge` e `login` por DOM visível e redirecionamento; após a
  confirmação, reabre a URL originalmente solicitada no mesmo perfil e recusa
  a retomada se ela voltar ao login;
- aceita a dica explícita `requires_login` + `login_url` no pedido de extração;
  a página de autenticação pode usar outro host, mas é aberta no perfil
  persistente pertencente à URL da fonte e a URL original continua sendo
  reaberta antes da extração;
- no perfil portátil, reutiliza o perfil Chromium isolado por hostname; cookies
  e sessões concedidos legitimamente permanecem sob controle do próprio Chrome
  e duas sessões não abrem simultaneamente o mesmo perfil.

Estados públicos:

```text
pending -> verifying -> completed
   |            |
   +-> cancelled+-> pending (verificação recusada)
   +-> expired
```

O status público inclui `kind: challenge|login`, permitindo que o cliente
apresente instruções adequadas sem receber cookies, credenciais ou DOM interno.

O token funciona como uma capability secreta. Portanto, caminhos de challenge
são redigidos nos logs, respostas não podem ser armazenadas em cache e a página
usa CSP, `no-referrer`, `nosniff` e proteção contra framing externo.
