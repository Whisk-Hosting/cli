# Database: node-postgres and sslmode=require

SKILL.md §5 says how Node's `pg` (node-postgres, and Prisma's `@prisma/adapter-pg`) connects with
`DATABASE_URL`: it reads `sslmode=require` as `verify-full` and refuses the platform's
certificate, so code using `pg` adds `uselibpqcompat=true`, which only `pg` reads.
