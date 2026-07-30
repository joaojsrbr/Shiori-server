# Instruções para agentes — Shiori Backend

Antes de qualquer ação:

1. leia `docs/prompts/PROMPT.md`;
2. leia `docs/megabrain/README.md` e os documentos indicados por ele;
3. leia ADRs relacionados à tarefa;
4. inspecione `git status`, branch atual e histórico;
5. siga o workflow Git abaixo.

## Escopo

- Este repositório contém somente servidor e workers.
- Não altere o frontend a partir deste repositório.
- `api/openapi/shiori.yaml` é a fonte de verdade pública.
- Preserve alterações existentes e nunca inclua segredos ou dados portáteis.

## Primeiro commit do repositório

Se `main` ainda não tiver commits:

1. revise todos os arquivos e `git diff --no-index` quando necessário;
2. execute formatação, vet e testes;
3. confirme que builds, bancos, storage e segredos estão ignorados;
4. crie um baseline intencional em `main`:
   `chore: establish backend baseline`;
5. somente depois crie a branch da próxima implementação.

Não crie baseline se houver arquivos suspeitos, segredos ou alterações cuja
origem não possa ser determinada.

## Workflow Git obrigatório

### Preparação

```powershell
git status --short
git switch main
git switch -c feature/<slug-curto>
```

Tipos permitidos:

```text
feature/<slug>
fix/<slug>
refactor/<slug>
test/<slug>
docs/<slug>
chore/<slug>
```

`git checkout -b <branch>` pode ser usado como alternativa.

Se `main` estiver suja com trabalho não relacionado, preserve-o e não prossiga
como se fosse seu. Nunca use `reset --hard`, `checkout -- .` ou clean
destrutivo.

### Implementação

- Faça commits pequenos com Conventional Commits.
- Exemplos: `feat(extraction): add LM Studio adapter`,
  `fix(queue): recover expired leases`.
- Revise `git status`, `git diff` e `git diff --cached`.
- Atualize OpenAPI antes do código quando o contrato mudar.
- Atualize `docs/megabrain/` na mesma branch.
- Não versione `dist/`, SQLite, storage, logs, `.env` ou credenciais.

Antes do merge:

```powershell
gofmt -w .
go vet ./...
go test ./...
git diff --check
git status --short
```

Execute também smoke tests do `.exe`, Docker ou worker quando afetados.

### Conclusão

Com verificações aprovadas:

```powershell
git switch main
git merge --no-ff feature/<slug-curto>
go vet ./...
go test ./...
git branch -d feature/<slug-curto>
```

Use merge normal com `--no-ff`. Não faça squash, rebase de commits publicados,
force push ou merge remoto automaticamente. Push, pull request e alterações no
GitHub exigem pedido explícito.

Se houver mudanças pareadas no frontend, registre branch/contrato no Handoff;
cada repositório recebe seu próprio commit e merge.

## Critério de conclusão

Uma tarefa só termina quando:

- contrato, implementação, testes e MegaBrain estão coerentes;
- branch foi mesclada localmente em `main`;
- testes pós-merge passaram;
- branch local concluída foi removida;
- resultado, commits e verificações foram informados.

