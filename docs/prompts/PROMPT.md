# Prompt de implementação — Shiori Backend

Copie este documento integralmente para uma conversa nova com o Gemini 3.1 Pro
quando quiser construir o backend. O agente deve tratar este arquivo como
especificação executável e fonte de verdade da área `back/`.

## Missão

Você é o principal engenheiro backend do Shiori. Construa, dentro de `back/`,
um servidor pessoal em Go que indexa, organiza, arquiva e sincroniza mangás,
manhwas, quadrinhos, novels e animes. O backend deve expor um contrato estável
para o aplicativo Flutter, coordenar downloads e jobs, armazenar assets com
integridade e extrair dados estruturados de páginas usando regras
determinísticas e modelos NuExtract locais no LM Studio.

Implemente uma fatia vertical pequena por vez. Antes de escrever código:

1. leia `docs/prompts/PROMPT.md`, as regras em `.agents/` e o estado atual do
   repositório;
2. identifique a próxima entrega incompleta;
3. apresente plano, critérios de aceite, riscos e arquivos afetados;
4. aguarde aprovação somente se houver escolha irreversível ou falta de
   informação que altere materialmente o resultado;
5. implemente, formate, analise, teste e relate evidências reais.
6. **OBRIGATÓRIO:** Sempre escreva arquivos `_test.go` para toda e qualquer nova funcionalidade, garantindo a criação de mocks quando necessário para que não existam pacotes com `[no test files]`.

Não gere o produto inteiro em uma única execução.

## Resultado esperado

O backend deverá oferecer:

- API REST versionada e documentada por OpenAPI;
- WebSocket ou Server-Sent Events para progresso e eventos;
- biblioteca unificada por identidade canônica;
- fontes, capítulos, episódios, progresso e coleções;
- indexação em camadas, sem dependência exclusiva de IA;
- navegador automatizado isolado para páginas que exigem JavaScript;
- suporte responsável a páginas protegidas por Cloudflare;
- extração local por NuExtract3 e NuExtract 1.5 Tiny via LM Studio;
- downloads, deduplicação, manifestos, OCR, tradução e derivados;
- fila de jobs idempotente;
- SQLite, fila durável local e filesystem no executável portátil;
- PostgreSQL, Valkey/Redis e MinIO/S3 no Docker;
- observabilidade, backups, segurança e operação local reproduzível.
- distribuição nativa como executável portátil para Windows;
- imagens Docker e ambiente completo por Docker Compose.

## Limites

- Todo código do servidor fica em `back/`.
- Go é a linguagem do servidor principal.
- Um worker Playwright em TypeScript é permitido apenas em
  `back/workers/browser/`.
- O worker de navegador não contém regras de negócio e não acessa o banco
  diretamente.
- O domínio não importa HTTP, PostgreSQL, Redis/Valkey, S3, Playwright ou SDKs
  de IA.

## Stack e versões

Use:

- Go na versão estável registrada em `go.mod`;
- SQLite para dados transacionais do modo portátil;
- PostgreSQL para dados transacionais do modo Docker;
- fila durável em SQLite no portátil;
- Valkey no Docker, mantendo compatibilidade de protocolo com Redis;
- filesystem local no portátil;
- MinIO no Docker e interface compatível com S3 em produção;
- OpenAPI 3.1;
- migrations SQL versionadas;
- logs JSON estruturados;
- OpenTelemetry para tracing e métricas quando a fundação estiver estável;
- TypeScript + Playwright no worker de navegador;
- LM Studio em `http://127.0.0.1:1234` por padrão.

Não fixe versões arbitrárias. Pesquise as versões estáveis no momento da
implementação, registre-as e mantenha lockfiles.

### Perfis de infraestrutura

Implemente dois perfis explícitos atrás das mesmas interfaces:

| Capacidade | Perfil `portable` | Perfil `docker` |
| --- | --- | --- |
| Banco | SQLite | PostgreSQL |
| Fila | SQLite durável | Valkey |
| Assets | filesystem local | MinIO/S3 |
| API/worker Go | um único `.exe` | containers separados |
| Navegador | adapter Go + Chrome/Edge local | worker Playwright |
| IA | LM Studio externo | LM Studio externo/configurável |

O perfil é escolhido por `--profile portable|docker` ou `SHIORI_PROFILE`.
Nunca detecte silenciosamente a infraestrutura e nunca faça fallback automático
de PostgreSQL para SQLite após uma falha.

Repositórios, fila, storage e navegador devem usar interfaces comuns, com
adapters separados. Não espalhe condicionais de perfil pelo domínio ou pelos
handlers HTTP.

## Distribuição, executável portátil e Docker

O backend deve possuir dois formatos oficiais de distribuição, produzidos a
partir do mesmo código e cobertos pelos mesmos testes.

### 1. Um único executável portátil para Windows

O artefato portátil oficial é um único arquivo:

```text
shiori-server.exe
```

Não distribua junto DLL própria, Node.js, migrations soltas, scripts,
configuração obrigatória ou outro executável. No primeiro início, o próprio
servidor cria ao lado do `.exe`:

```text
shiori-server.exe
data/
└── shiori.db
storage/
backups/
logs/
tmp/
```

O executável deve:

- iniciar com duplo clique ou `.\shiori-server.exe serve --profile portable`;
- usar `portable` como perfil padrão no build Windows;
- oferecer `version`, `doctor`, `migrate`, `backup` e `restore`;
- funcionar sem Docker, PostgreSQL, Valkey ou MinIO;
- funcionar sem instalação e sem escrever no Registro do Windows;
- resolver a pasta base por `os.Executable()`, nunca pelo diretório atual;
- permitir mudança explícita de diretório por `--data-dir`;
- criar diretórios somente quando necessários;
- encerrar corretamente com Ctrl+C ou sinal do sistema;
- abrir logs no console e em `logs/`, com rotação;
- iniciar mesmo sem LM Studio ou navegador, marcando apenas essas capabilities
  como indisponíveis.

Mover `shiori-server.exe` junto com `data/`, `storage/` e `backups/` deve
preservar o funcionamento. Guarde storage keys relativas, nunca caminhos
absolutos da máquina.

#### SQLite portátil

O banco padrão é:

```text
<diretório-do-exe>/data/shiori.db
```

O adapter SQLite deve:

- usar driver Go compatível com `CGO_ENABLED=0`, quando tecnicamente viável;
- habilitar foreign keys;
- usar WAL quando suportado pelo filesystem;
- configurar `busy_timeout`;
- limitar e documentar concorrência de escrita;
- executar migrations SQLite embutidas e versionadas;
- impedir duas instâncias gravando na mesma pasta;
- validar integridade no comando `doctor`;
- fazer backup consistente pela API de backup do SQLite ou equivalente;
- recusar compartilhamentos de rede não suportados;
- oferecer checkpoint e compactação controlados.

A fila portátil também fica no SQLite, com idempotency key, lease, heartbeat,
tentativas e recuperação após reiniciar o `.exe`. O filesystem local substitui
MinIO nesse perfil.

#### Navegador no executável único

O portátil não inclui Node ou Playwright. Implemente uma porta
`BrowserProvider` e um adapter Go compilado no `.exe`, usando Chrome ou Edge já
instalado na máquina:

- localizar executável do navegador por configuração e caminhos conhecidos;
- usar perfil persistente isolado em `data/browser-profiles/`;
- abrir janela visível quando houver `requires_user_action`;
- permitir que o usuário conclua challenge, CAPTCHA ou login;
- nunca reutilizar o perfil pessoal padrão do usuário;
- não automatizar CAPTCHA nem técnicas de evasão;
- desativar somente a capability de navegador se Chrome/Edge não estiver
  disponível.

No Docker, a mesma porta será atendida pelo worker Playwright.

Prefira dependências Go puras para permitir:

```powershell
$env:CGO_ENABLED = "0"
$env:GOOS = "windows"
$env:GOARCH = "amd64"
go build -trimpath -buildvcs=false -o dist\shiori-server.exe .\cmd\api
```

Centralize o build em `back/scripts/build-portable.ps1` ou em um task runner.
Não dependa de um comando manual não documentado. Injete por `-ldflags`:

- versão semântica;
- commit;
- data de build;
- indicador de árvore modificada.

O comando `version` deve expor esses campos. Builds reproduzíveis não devem
incorporar caminhos absolutos da máquina.

Use `go:embed`, quando adequado, para manter dentro do `.exe`:

- migrations SQLite;
- OpenAPI;
- templates internos;
- configuração padrão sem segredos.

Mesmo embutidos, migrations e OpenAPI devem continuar disponíveis como arquivos
fonte no repositório. Não embuta `.env`, tokens, certificados, cookies, modelos
de IA, executáveis de terceiros ou navegadores.

Migrations PostgreSQL permanecem no repositório e na imagem Docker:

```text
migrations/
├── sqlite/
└── postgres/
```

Os dois dialetos devem representar o mesmo modelo lógico e passar pelos mesmos
testes de comportamento dos repositórios.

### 2. Docker

Forneça Dockerfiles multi-stage:

```text
back/
├── Dockerfile
├── compose.yaml
└── workers/browser/Dockerfile
```

O `back/Dockerfile` deve:

- compilar em uma etapa Go fixada por versão;
- copiar somente o binário e certificados necessários para a imagem final;
- executar como usuário sem privilégios;
- usar filesystem read-only quando possível;
- possuir `HEALTHCHECK` baseado em `/health/ready`;
- definir `ENTRYPOINT` explícito;
- aceitar configuração por ambiente e secrets montados;
- não incluir código-fonte, cache do compilador ou credenciais na imagem final;
- suportar shutdown gracioso e sinais de container;
- incluir labels OCI com versão, commit, origem e licença.

O worker Playwright deve ter imagem separada. Use uma base oficial compatível
com a versão do Playwright instalada e mantenha servidor e navegador em
processos/containers distintos.

O Compose completo deve oferecer:

```text
shiori-api
shiori-worker
shiori-browser-worker
postgres
valkey
minio
```

Requisitos:

- health checks reais;
- `depends_on` condicionado a serviços saudáveis quando suportado;
- volumes nomeados para PostgreSQL, MinIO e dados persistentes;
- rede interna para infraestrutura;
- publicar somente as portas necessárias;
- limites e política de restart configuráveis;
- profiles para serviços opcionais, especialmente browser e observabilidade;
- `.env.example` sem segredos;
- nenhum password padrão adequado a produção;
- migrations executadas de forma idempotente antes da API ficar pronta.

Forneça dois fluxos completamente independentes:

```powershell
# Portátil Windows: um único arquivo, sem Docker
.\shiori-server.exe serve --profile portable

# Docker: PostgreSQL, Valkey, MinIO e Playwright
docker compose up -d --build
```

Quando o servidor estiver em Docker e o LM Studio estiver no Windows host, a
configuração de desenvolvimento deve aceitar:

```text
SHIORI_LMSTUDIO_BASE_URL=http://host.docker.internal:1234
```

Não fixe esse endereço como padrão universal: em Linux, produção remota ou
outra topologia ele deve ser configurável. Documente que o LM Studio precisa
escutar em uma interface acessível ao container e que autenticação por token é
recomendada.

### Automação de release

Crie comandos únicos e reproduzíveis:

```text
build
test
build-portable-exe
docker-build
release
```

O pipeline de release deve:

1. executar testes, vet e análise estática;
2. gerar OpenAPI e verificar diff;
3. compilar uma única vez o `shiori-server.exe`;
4. executar smoke test do `.exe` em uma pasta vazia;
5. confirmar criação de `data/shiori.db` ao lado do `.exe`;
6. construir e testar as imagens Docker separadamente;
7. gerar SHA-256 e SBOM do `.exe` sem empacotá-lo com outros arquivos;
8. publicar somente se todas as verificações passarem.

Nunca publique artefatos com configuração real, cookies, dados, modelos locais
ou tokens do LM Studio.

## Estrutura desejada

Comece com um monólito modular:

```text
back/
├── cmd/
│   ├── api/
│   └── worker/
├── internal/
│   ├── platform/
│   │   ├── config/
│   │   ├── database/
│   │   ├── events/
│   │   ├── httpserver/
│   │   ├── logging/
│   │   ├── queue/
│   │   └── storage/
│   ├── identity/
│   ├── media/
│   ├── sources/
│   ├── library/
│   ├── chapters/
│   ├── progress/
│   ├── assets/
│   ├── extraction/
│   ├── jobs/
│   ├── translation/
│   ├── sync/
│   └── backup/
├── api/
│   └── openapi/
├── migrations/
│   ├── sqlite/
│   └── postgres/
├── workers/
│   └── browser/
├── scripts/
│   ├── build-portable.ps1
│   └── smoke-test.ps1
├── test/
├── Dockerfile
├── compose.yaml
└── .env.example
```

Dentro de cada módulo, separe domínio, aplicação, portas e adapters apenas
quando houver código real. Evite uma árvore cerimonial vazia.

## Contrato entre frontend e backend

O arquivo OpenAPI é a fonte de verdade. Toda alteração pública deve:

1. atualizar o contrato;
2. manter IDs, datas, paginação, enums e erros consistentes;
3. gerar ou atualizar o cliente Dart;
4. incluir teste de contrato;
5. documentar compatibilidade e migração.

Convenções:

- base path `/api/v1`;
- UUID ou ULID opaco como ID externo;
- timestamps ISO 8601 em UTC;
- paginação por cursor;
- `application/problem+json` para erros;
- `Idempotency-Key` para comandos repetíveis;
- `X-Request-ID` para correlação;
- ETag/If-Match quando houver concorrência de edição.

Endpoints iniciais:

```text
GET    /health/live
GET    /health/ready
GET    /api/v1/capabilities
GET    /api/v1/media
POST   /api/v1/media
GET    /api/v1/media/{mediaId}
GET    /api/v1/media/{mediaId}/chapters
POST   /api/v1/sources/analyze
POST   /api/v1/sources/{sourceId}/index
GET    /api/v1/jobs
GET    /api/v1/jobs/{jobId}
POST   /api/v1/jobs/{jobId}/cancel
GET    /api/v1/ai/models
POST   /api/v1/ai/models/{modelKey}/load
GET    /api/v1/events
```

Não implemente todos de uma vez. Comece por health, capabilities e uma fatia
mínima de biblioteca.

## Domínio mínimo

Modele explicitamente:

- `Media`: identidade canônica, tipo, títulos, descrição, autores, gêneros,
  status e identificadores externos;
- `Source`: domínio, idioma, confiabilidade, capacidades e política de acesso;
- `SourceMedia`: vínculo entre obra canônica e página de uma fonte;
- `Chapter`/`Episode`: número normalizado, título, idioma e fonte;
- `Asset`: hash, tamanho, MIME verificado, dimensões e storage key;
- `ChapterManifest`: versão imutável e lista ordenada de assets;
- `LibraryEntry`: estado `library`, `tracking` ou `archived`;
- `ReadingProgress`;
- `ExtractionRule`: versão, seletores, confiança e última validação;
- `Job`: tipo, estado, progresso, tentativas e erro normalizado.

Não use float para números de capítulo quando isso perder sufixos como 10.5,
10a, extra ou especial. Guarde valor original e uma chave normalizada.

## Pipeline de aquisição e Cloudflare

O backend deve acessar páginas por uma política em camadas:

```text
URL solicitada
  -> validação SSRF e política da fonte
  -> HTTP direto, quando permitido
  -> detecção de conteúdo insuficiente/bloqueio
  -> worker Playwright com navegador real e JavaScript
  -> desafio interativo, quando necessário
  -> snapshot sanitizado + metadados de aquisição
  -> pipeline de extração
```

### Política de Cloudflare

“Passar pelo Cloudflare” significa conseguir usar fontes autorizadas atrás de
Cloudflare por meio de uma sessão real de navegador. Não significa derrotar
proteções.

Implemente:

- contexto Playwright persistente e isolado por usuário e domínio;
- cookies criptografados em repouso;
- armazenamento separado de sessão por fonte;
- carregamento de JavaScript e espera por sinais específicos da página;
- detecção de challenge page, 403, 429, CAPTCHA e login;
- estado de job `requires_user_action`;
- fluxo de handoff para o frontend abrir a sessão e o usuário concluir o
  desafio ou login;
- retomada do job depois da confirmação;
- rate limit, backoff, jitter e circuit breaker por domínio;
- identificação honesta do cliente e respeito a `Retry-After`;
- auditoria sem registrar cookies, tokens ou conteúdo sensível.

Não implemente:

- resolução automática de CAPTCHA;
- fingerprint spoofing agressivo;
- rotação de proxy para evasão;
- reutilização de sessão entre usuários;
- tentativa infinita de challenge;
- ocultação da automação para violar termos.

O worker Playwright recebe um comando restrito, devolve DOM/snapshot, URL
final, status, headers permitidos e lista de assets observados. O Go valida
tudo novamente.

## Pipeline de extração

Use a seguinte ordem:

1. plugin/regra específica da fonte;
2. JSON-LD, OpenGraph e metatags;
3. seletores versionados conhecidos;
4. heurísticas determinísticas;
5. NuExtract local;
6. revisão ou correção assistida.

Nunca envie HTML bruto ilimitado ao modelo. O pipeline deve:

- preservar JSON-LD e metatags relevantes;
- identificar `main`, conteúdo semântico e listas de capítulos;
- remover scripts, estilos, navegação e conteúdo invisível irrelevante;
- resolver URLs relativas contra a URL final;
- canonicalizar e limitar atributos;
- dividir por blocos semânticos, não por corte cego de caracteres;
- preservar capítulos inteiros dentro do mesmo bloco;
- aplicar limites de bytes, tokens, tempo e quantidade de itens;
- armazenar o hash do input sanitizado e a versão do extrator;
- validar a saída;
- consolidar chunks de forma determinística;
- registrar proveniência por campo.

Use `D:\projetos\nuextract-test\extract_data.py` apenas como protótipo de
referência. Corrija no produto:

- o corte de HTML por tamanho, que pode quebrar tags e registros;
- a ausência de consolidação e deduplicação de chunks;
- o model ID fixo;
- a falta de timeout e cancelamento HTTP;
- a validação permissiva com `additionalProperties: true`;
- a impressão de conteúdo potencialmente sensível;
- o tratamento genérico de exceções;
- a tentativa de evasão com `cloudscraper`.

## LM Studio e modelos NuExtract

Existem três model IDs configuráveis:

```text
nuextract3@q4_k_m
nuextract3@q5_k_m
nuextract-1.5-tiny
```

Nunca espalhe esses nomes pelo código. Centralize:

```text
SHIORI_LMSTUDIO_BASE_URL=http://127.0.0.1:1234
SHIORI_LMSTUDIO_API_TOKEN=
SHIORI_MODEL_EXTRACT_TINY=nuextract-1.5-tiny
SHIORI_MODEL_EXTRACT_DEFAULT=nuextract3@q4_k_m
SHIORI_MODEL_EXTRACT_QUALITY=nuextract3@q5_k_m
```

Descubra IDs e estado real por `GET /api/v1/models`. Use a API nativa do
LM Studio para gestão:

```text
GET  /api/v1/models
POST /api/v1/models/load
POST /api/v1/models/unload
```

Use o endpoint OpenAI-compatible `POST /v1/chat/completions` para inferência
NuExtract, pois o adapter precisa enviar:

```json
{
  "chat_template_kwargs": {
    "template": "{...template NuExtract...}",
    "instructions": "...",
    "enable_thinking": false
  }
}
```

Não construa manualmente tokens especiais de chat quando o runtime puder
aplicar o template oficial.

### Roteador de modelos

Use:

- **Tiny:** descoberta rápida, extração preliminar de HTML limpo, classificação
  de página, confirmação de campos simples e processamento em lote econômico;
- **Q4_K_M:** extrator padrão para páginas de obra e listas de capítulos;
- **Q5_K_M:** fallback de qualidade para baixa confiança, schema inválido,
  layout complexo, conteúdo multilíngue difícil ou divergência entre regras.

Fluxo padrão:

```text
determinístico
  -> suficiente? finalizar
  -> tiny opcional para classificação
  -> Q4 sem thinking
  -> validar + calcular confiança
  -> falhou/ambíguo? Q5
  -> continua ambíguo? requires_review
```

Não execute Q4 e Q5 sempre. Evite carregar os três simultaneamente quando a
memória for limitada. O gerenciador deve suportar JIT load, idle TTL,
auto-evict, timeout, retry limitado e health check.

NuExtract3:

- use template cuja estrutura corresponda ao JSON de saída;
- use `verbatim-string` quando o texto exato importar;
- use `date-time`, `integer`, `number`, enums e arrays corretamente;
- espere `null` ou `[]` quando um campo não for encontrado;
- comece com `enable_thinking=false` e temperatura 0.2;
- habilite thinking somente no Q5 e apenas para casos difíceis;
- aceite texto, imagem ou ambos quando o runtime/modelo carregado suportar.

NuExtract 1.5 Tiny é text-only. Não envie imagens a ele.

### Template inicial de mídia

```json
{
  "title": "verbatim-string",
  "alternative_titles": ["verbatim-string"],
  "description": "string",
  "cover_url": "verbatim-string",
  "authors": ["verbatim-string"],
  "artists": ["verbatim-string"],
  "status": ["ongoing", "completed", "hiatus", "cancelled", "unknown"],
  "genres": ["verbatim-string"],
  "chapters": [
    {
      "number_raw": "verbatim-string",
      "title": "verbatim-string",
      "url": "verbatim-string",
      "published_at": "date-time"
    }
  ]
}
```

Valide com JSON Schema estrito, `additionalProperties: false`, limites de
array e formatos. Normalize depois da validação, preservando o valor original.

## Jobs

Jobs iniciais:

```text
ANALYZE_SOURCE
FETCH_PAGE
INDEX_MEDIA
INDEX_CHAPTERS
DOWNLOAD_CHAPTER
PROCESS_IMAGE
RUN_OCR
TRANSLATE_PAGE
CHECK_UPDATES
MATCH_MEDIA
REPAIR_SOURCE
CREATE_BACKUP
```

Estados:

```text
queued
running
requires_user_action
retry_scheduled
succeeded
succeeded_with_warnings
failed
cancelled
```

Cada job precisa de idempotency key, lease, heartbeat, progresso estruturado,
tentativas máximas, backoff, cancelamento cooperativo e dead-letter.

## Assets e armazenamento

- Calcule SHA-256 durante streaming.
- Verifique MIME por conteúdo, não apenas extensão/header.
- Imponha limites antes e durante o download.
- Grave primeiro em staging e promova atomicamente.
- Nunca use URL ou título como caminho físico.
- Deduplicate por hash.
- Separe original, thumbnail, otimizado, OCR e tradução.
- Crie manifesto de capítulo imutável e versionado.
- Faça garbage collection apenas por marcação e varredura, com período de
  segurança.

## Segurança

Obrigatório:

- parsing de URL e bloqueio de IPs privados/reservados para evitar SSRF;
- allowlist explícita quando uma fonte precisar acessar hosts auxiliares;
- proteção contra DNS rebinding;
- timeouts e limites de conexão/resposta;
- queries parametrizadas;
- autenticação e autorização por capacidade;
- cookies e tokens criptografados;
- segredos somente em ambiente/secret store;
- sanitização de logs;
- sandbox e permissões mínimas para plugins;
- assinatura ou confiança explícita de plugins;
- validação de arquivo e proteção contra decompression bombs;
- CORS restrito;
- CSRF quando autenticação usar cookies;
- rate limit por usuário, rota e domínio externo.

Trate páginas, plugins e saídas de IA como dados não confiáveis.

## Observabilidade

Inclua:

- request ID, job ID, source ID e model key;
- latência por etapa;
- status de aquisição;
- tokens, tempo até primeiro token e tokens/s quando disponíveis;
- modelo e quantização usados;
- taxa de fallback Tiny -> Q4 -> Q5;
- erros de schema;
- desafios Cloudflare e ações do usuário, sem segredos;
- métricas de fila, storage e banco.

Não registre HTML completo, prompts com dados privados, cookies ou respostas
brutas por padrão. Disponibilize diagnóstico redigido e opt-in.

## Testes

Use:

- testes de tabela no Go;
- testes de domínio sem infraestrutura;
- testes do adapter SQLite com migrations reais;
- testes de repositório com PostgreSQL real em container;
- testes equivalentes dos repositórios SQLite e PostgreSQL;
- testes da fila SQLite depois de crash, reinício e expiração de lease;
- smoke test que copia somente o `.exe` para uma pasta vazia;
- testes de contrato OpenAPI;
- fixtures HTML locais;
- fake server do LM Studio;
- casos de JSON válido, inválido, truncado e com markdown fences;
- casos de timeout, cancelamento e modelo indisponível;
- casos de Cloudflare challenge como estado, sem tentar resolver;
- testes do consolidator com capítulos duplicados e chunks sobrepostos;
- testes de segurança para SSRF e path traversal;
- testes end-to-end mínimos.

Execute:

```powershell
gofmt -w .
go vet ./...
go test ./...
```

No worker:

```powershell
npm run format
npm run lint
npm test
```

## Primeiras entregas

Faça nesta ordem:

1. ADRs curtos: module path, router HTTP, perfis SQLite/PostgreSQL, migrations,
   filas, navegadores e geração OpenAPI;
2. `go.mod`, config tipada, logging, shutdown e health checks;
3. OpenAPI mínimo e tratamento padronizado de erros;
4. SQLite, fila SQLite, filesystem e adapter de navegador Go;
5. build único do `.exe`, comando `version` e smoke test portátil;
6. PostgreSQL, Valkey, MinIO, Dockerfile e Compose;
7. mídia/biblioteca mínima persistida nos dois perfis;
8. interface `ExtractionProvider` e fake testável;
9. adapter LM Studio com list/load/infer e três aliases;
10. sanitizador semântico, schema e consolidador;
11. job `ANALYZE_SOURCE`;
12. worker Playwright Docker e fluxo `requires_user_action`.

Em cada conversa, implemente somente a próxima entrega coerente.

## Critério de pronto

Uma entrega está pronta somente quando:

- compila;
- contrato e migrations estão coerentes;
- formatador, vet e testes afetados passam;
- erros e cancelamento foram tratados;
- não há segredo no repositório;
- existe instrução reproduzível de execução;
- somente o `.exe` é necessário no início do smoke test portátil;
- o `.exe` cria `data/shiori.db` ao lado de si;
- reiniciar o `.exe` preserva biblioteca e jobs;
- mover o `.exe` com suas pastas de dados preserva referências relativas;
- SQLite e PostgreSQL passam nos mesmos testes de comportamento;
- a imagem Docker inicia como usuário sem privilégios e fica saudável;
- o Compose documentado sobe ou seus impedimentos são relatados com evidência;
- hashes dos pacotes gerados são verificáveis;
- evidências reais foram apresentadas;
- riscos restantes e próximo passo estão documentados.

## Referências obrigatórias

- NuExtract3: `https://huggingface.co/numind/NuExtract3`
- NuExtract 1.5 Tiny:
  `https://huggingface.co/numind/NuExtract-1.5-tiny`
- LM Studio REST API: `https://lmstudio.ai/docs/developer/rest`
- Protótipo local:
  `D:\projetos\nuextract-test\extract_data.py`
