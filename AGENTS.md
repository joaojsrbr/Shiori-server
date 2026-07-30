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

Toda implementação forma uma árvore de branches: **blocos** pequenos são
mesclados na branch da **feature**, e a feature — só depois de completa e
validada — é mesclada em `main`. Cada branch de bloco existe apenas até ser
mesclada; ela é excluída logo em seguida.

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

### Divisão em blocos

Antes de codificar, quebre a implementação em blocos pequenos e coerentes
(ex.: "estrutura base do adapter", "integração com a fila", "testes de
contrato"). Cada bloco vira uma sub-branch da feature, nomeada:

```text
feature/<slug-curto>/bloco-<n>-<slug-do-bloco>
```

(o mesmo padrão se aplica a fix/, refactor/, test/, docs/, chore/).

Para cada bloco:

```powershell
git switch feature/<slug-curto>
git switch -c feature/<slug-curto>/bloco-<n>-<slug-do-bloco>
```

- Implemente somente o escopo do bloco.
- Faça commits pequenos com Conventional Commits, referenciando o bloco
  quando útil: `feat(extraction): add LM Studio adapter base (bloco 1/3)`.
- Revise `git status`, `git diff` e `git diff --cached`.
- Atualize OpenAPI antes do código quando o contrato mudar.
- Atualize `docs/megabrain/` conforme o bloco avança.
- Não versione `dist/`, SQLite, storage, logs, `.env` ou credenciais.

Ao concluir o bloco, valide e mescle de volta na branch da feature:

```powershell
gofmt -w .
go vet ./...
go test ./...
git switch feature/<slug-curto>
git merge --no-ff feature/<slug-curto>/bloco-<n>-<slug-do-bloco>
git branch -d feature/<slug-curto>/bloco-<n>-<slug-do-bloco>
```

Repita para cada bloco seguinte, sempre criando a sub-branch a partir da
branch da feature já atualizada. O resultado é uma árvore local: blocos →
feature → main. Branches de bloco não são publicadas — push, pull request e
alterações no GitHub exigem pedido explícito, como no restante do workflow.

### Validação da feature completa

Com todos os blocos mesclados na branch da feature:

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

Use merge normal com `--no-ff` em cada nível da árvore (bloco → feature,
feature → main). Não faça squash, rebase de commits publicados, force push ou
merge remoto automaticamente.

Se houver mudanças pareadas no frontend, registre branch/contrato no Handoff;
cada repositório recebe seu próprio commit e merge.

## Critério de conclusão

Uma tarefa só termina quando:

- contrato, implementação, testes e MegaBrain estão coerentes;
- todos os blocos foram mesclados na branch da feature e suas branches
  removidas;
- a branch da feature foi mesclada localmente em `main`;
- testes pós-merge passaram;
- todas as branches locais concluídas (blocos e feature) foram removidas;
- resultado, commits e verificações foram informados.
