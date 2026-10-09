package doctor

import (
	"strings"
	"testing"
)

// W105 on the passing fixture: only with database_role restricted, for a role switch anywhere in
// server code and a schema change outside the migrations.
func TestRunLoginRule(t *testing.T) {
	restricted := passing["whisk.yaml"] + "database_role: restricted\nmigrate: \"node dist/migrate.js\"\n"
	if passing["whisk.yaml"] == "" {
		t.Fatal("the passing fixture has no whisk.yaml")
	}
	cases := []struct {
		name  string
		files map[string]string
		want  bool
	}{
		{"nothing to find", map[string]string{"whisk.yaml": restricted}, false},
		{"set local role in a query", map[string]string{"whisk.yaml": restricted, "src/db.ts": "await tx.$executeRawUnsafe(`SET LOCAL ROLE app_rls`);\n"}, true},
		{"create table at boot", map[string]string{"whisk.yaml": restricted, "src/boot.ts": "await sql`create table if not exists sessions (id text)`;\n"}, true},
		{"python alter table outside migrations", map[string]string{"whisk.yaml": restricted, "app/setup.py": "cur.execute(\"ALTER TABLE orders ADD COLUMN x int\")\n"}, true},
		{"a schema change in a migration file", map[string]string{"whisk.yaml": restricted, "src/migrate.ts": "await sql`create table orders (id int)`;\n"}, false},
		{"a schema change in a comment", map[string]string{"whisk.yaml": restricted, "src/notes.ts": "// create table orders is done by the migration\nexport const x = 1;\n"}, false},
		{"reset role is not a switch", map[string]string{"whisk.yaml": restricted, "src/db.ts": "await sql`reset role`;\n"}, false},
		{"the owner may do both", map[string]string{"src/db.ts": "await sql`set local role app_rls`; await sql`create table t (id int)`;\n"}, false},
		{"allowed with a reason", map[string]string{"whisk.yaml": restricted, "src/diag.ts": "// doctor: allow W105 a diagnostic for an owner app\nawait sql`create table if not exists diag (id int)`;\n"}, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			rep := runDoctor(t, with(passing, c.files), false)
			if got := has(rep, "W105"); got != c.want {
				t.Errorf("W105 = %v, want %v; findings %+v; skipped %v", got, c.want, rep.Findings, rep.Skipped)
			}
		})
	}
}

// W102's finding says a migration at start stops a restricted app from starting.
func TestMigrateAtStartUnderRunLogin(t *testing.T) {
	files := with(passing, map[string]string{
		"whisk.yaml": passing["whisk.yaml"] + "database_role: restricted\nmigrate: \"node dist/migrate.js\"\n",
		"Dockerfile": "FROM node:22-slim\nWORKDIR /app\nCOPY . .\nUSER node\nEXPOSE 8080\nCMD [\"sh\", \"-c\", \"npx prisma migrate deploy && node dist/index.js\"]\n",
	})
	rep := runDoctor(t, files, false)
	found := false
	for _, f := range rep.Findings {
		if f.Rule == "W102" {
			found = true
			if !strings.Contains(f.Message, "fails to start") {
				t.Errorf("W102 message does not say the app fails to start: %s", f.Message)
			}
		}
	}
	if !found {
		t.Errorf("no W102 finding: %+v", rep.Findings)
	}
}
