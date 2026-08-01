# Handoff atual

## Próximo objetivo recomendado

Iniciar o desenvolvimento do Frontend (Flutter). O Backend atingiu maturidade de fundação (Pipeline de NuExtract, Worker Pool assíncrono, Repositórios e Testes concluídos).

## Sequência

1. Navegar para `front/`;
2. Consultar o MegaBrain do Frontend (`front/docs/megabrain/README.md`);
3. Iniciar a UI (Gerenciamento de Estado, Roteamento) baseada no `openapi/shiori.yaml`.

## Atenções

- Não versionar `dist/shiori-server.exe`.
- Confirmar compatibilidade da versão Go declarada com o ambiente.
- PostgreSQL/Valkey/MinIO ainda pertencem ao perfil Docker.
- Ao mudar a janela carregada no LM Studio, alinhar
  `SHIORI_AI_MAX_CONTEXT_LENGTH`; o padrão é `8192`.
- A rota `/api/v1/debug/extract` retorna 404 fora de `--log-level debug`.
- O frontend deve resolver `challenge_url` relativo à URL do backend, abrir a
  página temporária e acompanhar `GET /api/v1/challenges/{token}/status`; não
  deve acessar cookies ou CDP diretamente.

