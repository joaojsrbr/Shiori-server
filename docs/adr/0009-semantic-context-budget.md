# ADR-009: Orçamento de contexto e chunking semântico

**Status:** Aceito
**Data:** 2026-08-01

## Contexto

O HTML renderizado pode exceder a janela realmente carregada no LM Studio. O
corte anterior usava bytes fixos, aceitava uma linha individual acima do limite
e reservava `16384` tokens de saída; em um modelo com contexto de `8192`, isso
produzia HTTP 400 antes da inferência. Cortes cegos também separam títulos das
listas de capítulos e prejudicam a consolidação.

## Decisão

O pipeline passa a:

- preservar metadados de alto sinal (`title`, OpenGraph e JSON-LD limitado);
- remover navegação, scripts comuns, estilos, formulários e conteúdo decorativo;
- resolver `href` e `src` relativos contra a URL final do navegador;
- converter o DOM limpo em Markdown;
- formar blocos por títulos e separadores semânticos;
- compactar blocos até um teto rígido, deduplicando blocos idênticos;
- repetir o título corrente como breadcrumb, sem overlap arbitrário;
- dividir blocos excedentes por linha e, apenas como último recurso, em uma
  fronteira UTF-8 válida;
- calcular o teto de entrada a partir da janela configurada, do template, de
  overhead conservador e da reserva de saída;
- serializar documento e template no formato GGUF do NuExtract porque o
  endpoint OpenAI-compatible do LM Studio não expõe `chat_template_kwargs`.

Como a API OpenAI-compatible do LM Studio não fornece o tokenizer do modelo, a
estimativa usa três bytes por token. Os limites continuam configuráveis por
`SHIORI_AI_MAX_CONTEXT_LENGTH` e `SHIORI_AI_MAX_CONTENT_BYTES`.

## Consequências

- Nenhuma requisição solicita mais tokens de saída que a janela inteira.
- Cada chunk respeita um limite rígido inclusive com linhas enormes e Unicode.
- Extrações longas exigem várias inferências e podem levar mais tempo.
- A consolidação permanece determinística e deduplica listas ao final.
- Trocar o contexto no LM Studio exige alinhar a configuração do Shiori.

## Referências

- NuExtract3 model card: structured extraction e formato de template.
- LM Studio Chat Completions: parâmetros aceitos pelo endpoint compatível.
- Unstructured: estratégias `basic` e `by_title`, limites soft/hard.
- `golang.org/x/net/html`: parser HTML5 para construir a árvore DOM.
