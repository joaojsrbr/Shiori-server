# ADR-001: Module Path Go

**Status:** Aceito  
**Data:** 2026-07-30

## Contexto

O backend Shiori precisa de um module path Go estável que será usado em
`go.mod`, importações internas e, eventualmente, como referência pública se o
repositório for aberto.

O path deve:

- ser curto e memorável;
- funcionar com `go install` e `go get`;
- não depender da estrutura de diretórios local;
- não usar caminhos provisórios como `example.com`.

## Decisão

O module path será:

```
github.com/joaojsr/shiori-server
```

O `go.mod` será inicializado com:

```
module github.com/joaojsr/shiori-server

go 1.26
```

## Consequências

- Todo import interno segue o padrão
  `github.com/joaojsr/shiori-server/internal/...`.
- O repositório GitHub deve coincidir com o module path para que `go install`
  funcione sem redirects.
- Se o repositório mudar de nome ou organização, será necessário manter um
  redirect ou atualizar o module path em uma major version bump.
- Pacotes dentro de `internal/` não serão importáveis externamente, conforme
  a convenção Go.
