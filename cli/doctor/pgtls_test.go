package doctor

import "testing"

func TestW107(t *testing.T) {
	cases := []struct {
		name  string
		files map[string]string
		want  bool
	}{
		{"pool on the address as it is", map[string]string{"src/db.ts": "import pg from \"pg\"\nexport const pool = new pg.Pool({ connectionString: process.env.DATABASE_URL })\n"}, true},
		{"client required", map[string]string{"src/backup.js": "const { Client } = require(\"pg\")\nconst c = new Client({ connectionString: url })\n"}, true},
		{"prisma adapter", map[string]string{"src/prisma.ts": "import { PrismaPg } from \"@prisma/adapter-pg\"\nconst adapter = new PrismaPg({ connectionString: process.env.DATABASE_URL })\n"}, true},
		{"libpq meaning set", map[string]string{"src/db.ts": "import pg from \"pg\"\nconst url = new URL(process.env.DATABASE_URL!)\nurl.searchParams.set(\"uselibpqcompat\", \"true\")\nexport const pool = new pg.Pool({ connectionString: url.toString() })\n"}, false},
		{"own tls options", map[string]string{"src/db.ts": "import { Pool } from \"pg\"\nexport const pool = new Pool({ connectionString: u, ssl: { rejectUnauthorized: false } })\n"}, false},
		{"another driver's pool", map[string]string{"src/db.ts": "import { Pool } from \"@neondatabase/serverless\"\nexport const pool = new Pool({ connectionString: u })\n"}, false},
		{"pg types only", map[string]string{"src/types.ts": "import type { PoolClient } from \"pg\"\nexport type Tx = PoolClient\n"}, false},
		{"no database", map[string]string{"whisk.yaml": replaceOnce(passing["whisk.yaml"], "database: app\n", "database: none\n"), "src/db.ts": "import pg from \"pg\"\nexport const pool = new pg.Pool({ connectionString: u })\n"}, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			rep := runDoctor(t, with(passing, c.files), false)
			if got := has(rep, "W107"); got != c.want {
				t.Errorf("W107 = %v, want %v; findings %+v", got, c.want, rep.Findings)
			}
		})
	}
}
