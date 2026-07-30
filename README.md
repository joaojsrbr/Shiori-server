# Shiori Back

Área reservada ao servidor Go e aos workers auxiliares.

Antes de implementar, leia [docs/prompts/PROMPT.md](docs/prompts/PROMPT.md).
A memória persistente do projeto fica em
[docs/megabrain/README.md](docs/megabrain/README.md).

Estrutura inicial planejada:

```text
back/
├── cmd/api/
├── internal/
├── migrations/
├── api/openapi/
└── workers/browser/
```

O módulo Go ainda não foi inicializado. Defina o module path como uma decisão
explícita na primeira entrega, em vez de usar um caminho provisório.
