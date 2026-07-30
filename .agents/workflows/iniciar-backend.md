# Iniciar entrega do backend

1. Leia `@/AGENTS.md`, `@/docs/prompts/PROMPT.md`, MegaBrain e ADRs.
2. Inspecione branch, histórico, status e alterações existentes.
3. Se `main` ainda não tiver commit, crie o baseline seguro descrito em
   `AGENTS.md`.
4. Crie `feature/<slug>` ou outro tipo permitido a partir de `main`.
5. Identifique a menor fatia vertical e quebre-a em blocos pequenos e
   coerentes.
6. Para cada bloco, crie `feature/<slug>/bloco-<n>-<slug-do-bloco>` a partir
   da branch da feature.
7. Atualize primeiro o OpenAPI quando o contrato público mudar.
8. Preserve equivalência comportamental entre SQLite e PostgreSQL.
9. Implemente sem acoplar domínio à infraestrutura.
10. Atualize o MegaBrain na mesma branch de bloco.
11. Formate, execute vet/testes/smoke tests, revise diff e faça commits do
    bloco.
12. Volte para a branch da feature, faça merge `--no-ff` do bloco e remova a
    branch do bloco.
13. Repita os passos 6–12 para cada bloco seguinte.
14. Com todos os blocos mesclados, formate, execute vet/testes/smoke tests
    completos na branch da feature.
15. Volte para `main`, faça merge `--no-ff` da feature e teste novamente.
16. Remova a branch local da feature e relate commits e evidências.
