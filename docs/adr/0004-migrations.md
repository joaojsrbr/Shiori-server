# ADR-004: Estratégia de Migrations

**Status:** Aceito  
**Data:** 2026-07-30

## Contexto

O Shiori usa dois bancos de dados (SQLite e PostgreSQL) que representam o
mesmo modelo lógico. As migrations devem:

- ser versionadas e auditáveis;
- suportar up e down;
- funcionar embutidas no executável portátil (SQLite);
- funcionar como arquivos na imagem Docker (PostgreSQL);
- executar de forma idempotente antes da API ficar pronta;
- ser testáveis em CI.

## Decisão

Usar **`golang-migrate/migrate`** v4.19+ com migrations SQL separadas por
dialeto.

### Estrutura

```
back/migrations/
├── sqlite/
│   ├── 000001_init.up.sql
│   ├── 000001_init.down.sql
│   ├── 000002_....up.sql
│   └── 000002_....down.sql
└── postgres/
    ├── 000001_init.up.sql
    ├── 000001_init.down.sql
    ├── 000002_....up.sql
    └── 000002_....down.sql
```

### Regras

1. Cada migration tem numeração sequencial de 6 dígitos e nome descritivo.
2. SQLite e PostgreSQL recebem migrations separadas para o mesmo schema lógico.
3. Diferenças de DDL (ex: `AUTOINCREMENT` vs `SERIAL`, `TEXT` vs `VARCHAR`)
   são expressas nos arquivos SQL, nunca em templates.
4. Migrations SQLite são embutidas via `go:embed` para uso no executável
   portátil.
5. Migrations PostgreSQL são copiadas para a imagem Docker.
6. Ambas continuam disponíveis como arquivos fonte no repositório.
7. O servidor executa migrations automaticamente no startup, antes de aceitar
   requests.
8. O comando `migrate` também pode ser executado manualmente.
9. Testes de comportamento validam que os dois dialetos produzem o mesmo
   comportamento nos repositórios.

### Alternativas descartadas

| Opção | Motivo da rejeição |
|---|---|
| goose | Menos extensível para múltiplos drivers e `go:embed` |
| GORM AutoMigrate | Controle insuficiente sobre DDL, índices e constraints |
| Atlas | Overhead de tooling; migrations declarativas menos auditáveis |
| SQL puro sem ferramenta | Sem tracking de versão, sem rollback |

## Consequências

- Toda alteração de schema exige dois arquivos SQL (um por dialeto).
- O time deve garantir equivalência lógica entre os dois conjuntos.
- Testes de repositório com ambos os bancos servem como safety net.
- O `golang-migrate` gerencia a tabela de versão (`schema_migrations`).
- Rollback é possível mas deve ser usado com cuidado em produção.
