# Handoff atual

## Próximo objetivo recomendado

Revisar a fundação existente, executar testes e criar o primeiro commit
baseline do backend antes de novas branches.

## Sequência

1. revisar arquivos não rastreados;
2. confirmar `.gitignore`;
3. executar `gofmt`, `go vet` e `go test`;
4. executar smoke test do `.exe` se o build estiver pronto;
5. criar `chore: establish backend baseline`;
6. abrir a próxima branch a partir de `main`.

## Atenções

- Não versionar `dist/shiori-server.exe`.
- Confirmar compatibilidade da versão Go declarada com o ambiente.
- PostgreSQL/Valkey/MinIO ainda pertencem ao perfil Docker.

