# Instruções para agentes — Shiori Backend

Antes de qualquer ação:

1. leia `docs/prompts/PROMPT.md`;
2. leia `docs/megabrain/README.md` e os documentos indicados por ele;
3. inspecione `git status`, branch atual e histórico;
4. siga o workflow Git abaixo.

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

O fluxo de trabalho Git oficial do projeto (válido para front e back) é:

1. A partir da branch `main`, **criar uma branch com o nome da feature** (ex: `feature/sua-tarefa`).
2. A partir **dessa branch nome (feature)**, você deve **criar as branches menores (os blocos)** para cada parte da tarefa.
3. No final de cada bloco concluído, você **mergeia a branch do bloco de volta na branch da feature** e exclui a branch do bloco.
4. Depois de finalizar e testar todos os blocos, no final da tarefa, você **mergeia a branch da feature na `main`**, excluindo a branch da feature, óbvio.

### 1. Criar a branch nome (feature) a partir da main

```powershell
git switch main
git switch -c feature/<slug-curto>
```

Se `main` estiver suja com trabalho não relacionado, preserve-o e não prossiga como se fosse seu. Nunca use `reset --hard`.

### 2. Criar e trabalhar nas branches (blocos)

A partir da sua branch `feature/<slug-curto>`, crie a branch do bloco:

```powershell
git switch feature/<slug-curto>
git switch -c feature/<slug-curto>/bloco-<n>-<slug-do-bloco>
```

- Implemente somente o escopo do bloco.
- Faça commits pequenos com Conventional Commits.
- Não versione `dist/`, SQLite, storage, logs, `.env` ou credenciais.

### 3. Fazer o merge do bloco na branch nome (feature)

Ao concluir o bloco, valide e mescle de volta na branch da feature:

```powershell
gofmt -w .
go vet ./...
go test ./...
git switch feature/<slug-curto>
git merge --no-ff feature/<slug-curto>/bloco-<n>-<slug-do-bloco>
git branch -d feature/<slug-curto>/bloco-<n>-<slug-do-bloco>
```

Repita o passo 2 e 3 para os próximos blocos. As branches de bloco são excluídas assim que mescladas.

### 4. No final, mergear na main e excluir a branch

Com todos os blocos mesclados na branch da feature, valide tudo:

```powershell
gofmt -w .
go vet ./...
go test ./...
git diff --check
git status --short
```

Com verificações aprovadas, faça o merge final na `main` e exclua a branch nome:

```powershell
git switch main
git merge --no-ff feature/<slug-curto>
go vet ./...
go test ./...
git branch -d feature/<slug-curto>
```

Use merge normal com `--no-ff` em cada nível (bloco → feature, feature → main).
Se houver mudanças pareadas no frontend, lembre-se que cada repositório tem seu próprio merge.

## Critério de conclusão

Uma tarefa só termina quando:

- contrato, implementação, testes e MegaBrain estão coerentes;
- todos os blocos foram mesclados na branch da feature e suas branches
  removidas;
- a branch da feature foi mesclada localmente em `main`;
- testes pós-merge passaram;
- todas as branches locais concluídas (blocos e feature) foram removidas;
- resultado, commits e verificações foram informados.
