# Shiori Backend

Este repositório contém exclusivamente o backend Go do Shiori.

Leia obrigatoriamente:

- `@/AGENTS.md`;
- `@/docs/prompts/PROMPT.md`;
- `@/docs/megabrain/README.md` e sua ordem de leitura;
- ADRs relacionados.

Toda implementação deve ser dividida em blocos pequenos, cada um em branch
própria mesclada `--no-ff` na branch da feature e removida em seguida; a
feature só é mesclada `--no-ff` em `main` depois que todos os blocos foram
integrados, conforme `AGENTS.md`.

## Limites

- Não altere arquivos do repositório `front`.
- O OpenAPI em `api/openapi/` é a fonte de verdade pública.
- O domínio não importa HTTP, banco, fila, storage, navegador ou SDK de IA.
- Trate páginas, plugins e respostas de IA como dados não confiáveis.
- Não implemente bypass de CAPTCHA, DRM, paywall, login ou Cloudflare.

## Perfis

`portable`:

- um único `shiori-server.exe`;
- SQLite em `data/shiori.db` ao lado do executável;
- fila durável no SQLite;
- assets no filesystem;
- adapter Go para Chrome/Edge instalado.

`docker`:

- PostgreSQL;
- Valkey;
- MinIO;
- worker Playwright separado.

Nunca faça fallback silencioso entre os perfis.

## NuExtract

- `nuextract-1.5-tiny`: classificação e extrações simples;
- `nuextract3@q4_k_m`: extrator padrão;
- `nuextract3@q5_k_m`: fallback de qualidade;
- LM Studio é acessado por adapter configurável;
- valide toda saída contra JSON Schema estrito.

## Verificação

Execute para mudanças Go:

```powershell
gofmt -w .
go vet ./...
go test ./...
```

Para release, valide separadamente o `.exe` portátil e as imagens Docker.
Não declare conclusão sem relatar os comandos realmente executados.
