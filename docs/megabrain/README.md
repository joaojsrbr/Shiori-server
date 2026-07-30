# MegaBrain — Shiori Backend

Memória técnica persistente para humanos e agentes. Registra estado real,
arquitetura, histórico, decisões e próximos passos do backend.

## Ordem obrigatória de leitura

1. `CURRENT_STATE.md`
2. `ARCHITECTURE.md`
3. `HANDOFF.md`
4. ADRs relacionados em `../adr/`
5. últimas entradas de `WORKLOG.md`
6. `../prompts/PROMPT.md` para a especificação completa

## Regras de manutenção

- Atualize o MegaBrain em toda implementação.
- Registre apenas fatos verificáveis, citando arquivos e testes.
- Mantenha `CURRENT_STATE.md` curto e substitua conteúdo obsoleto.
- Use ADR para decisões arquiteturais; aqui mantenha resumo e links.
- `WORKLOG.md` é append-only, salvo correção factual.
- Não cole logs extensos, prompts completos ou transcrições.
- Nunca registre tokens, cookies, URLs privadas, dados pessoais ou segredos.
- A atualização deve entrar na própria branch antes do merge.

