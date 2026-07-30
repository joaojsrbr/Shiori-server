# Iniciar entrega do backend

1. Leia `@/AGENTS.md`, `@/docs/prompts/PROMPT.md`, MegaBrain e ADRs.
2. Inspecione branch, histórico, status e alterações existentes.
3. Se `main` ainda não tiver commit, crie o baseline seguro descrito em
   `AGENTS.md`.
4. Crie `feature/<slug>` ou outro tipo permitido a partir de `main`.
5. Identifique a menor fatia vertical.
6. Atualize primeiro o OpenAPI quando o contrato público mudar.
7. Preserve equivalência comportamental entre SQLite e PostgreSQL.
8. Implemente sem acoplar domínio à infraestrutura.
9. Atualize o MegaBrain na mesma branch.
10. Formate, execute vet/testes/smoke tests, revise diff e faça commits.
11. Volte para `main`, faça merge `--no-ff` e teste novamente.
12. Remova a branch local concluída e relate commits e evidências.
