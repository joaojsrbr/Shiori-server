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
- encaminha somente eventos permitidos de ponteiro, roda do mouse e teclado;
- nunca expõe cookies, DevTools, session ID ou HTML interno ao cliente;
- aceita um único controlador de mesma origem por token;
- expira em três minutos e oferece cancelamento explícito;
- pede ao backend para verificar novamente o DOM antes de concluir;
- só retoma o job quando o challenge não estiver mais visível.

Estados públicos:

```text
pending -> verifying -> completed
   |            |
   +-> cancelled+-> pending (verificação recusada)
   +-> expired
```

O token funciona como uma capability secreta. Portanto, caminhos de challenge
são redigidos nos logs, respostas não podem ser armazenadas em cache e a página
usa CSP, `no-referrer`, `nosniff` e proteção contra framing externo.

## Limites

- Não resolve CAPTCHA automaticamente.
- Não injeta respostas, não altera fingerprint e não oculta automação.
- Não reutiliza o perfil pessoal do usuário.
- Persistência criptografada de cookies por domínio permanece uma entrega
  separada; a sessão atual é isolada e efêmera.
- O adapter Playwright do perfil Docker deverá implementar o mesmo contrato.

## Referências

- Chrome DevTools Protocol: `Page.startScreencast` e `Page.screencastFrame`.
- Chrome DevTools Protocol: `Input.dispatchMouseEvent` e
  `Input.dispatchKeyEvent`.
- Gorilla WebSocket: política de origem e regra de um leitor/um escritor.
- OWASP WebSocket Security Cheat Sheet.
