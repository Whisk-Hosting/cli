package doctor

import (
	"reflect"
	"strconv"
	"strings"
	"testing"
)

func TestShellCommands(t *testing.T) {
	cases := []struct {
		name, text string
		env        map[string]string
		want       []string
	}{
		{"one command", "node dist/index.js", nil, []string{"1 node dist/index.js"}},
		{"either side of && and ;", "npx prisma migrate deploy && node server.js; true", nil, []string{"1 npx prisma migrate deploy", "1 node server.js"}},
		{"words a command prints are not run", `echo "run prisma migrate deploy first" && exec node x.js`, nil, []string{"1 exec node x.js"}},
		{"inside if, at its line", "#!/bin/sh\nif [ -n \"$MIGRATE\" ]; then\n  alembic upgrade head\nfi\nexec uvicorn app.main:app\n", nil,
			[]string{"2 [ -n $MIGRATE ]", "3 alembic upgrade head", "5 exec uvicorn app.main:app"}},
		{"a shell's -c text", `sh -c "tsx src/index.ts"`, nil, []string{"1 tsx src/index.ts"}},
		{"a variable the Dockerfile sets", "node $APP", map[string]string{"APP": "dist/index.js"}, []string{"1 node dist/index.js"}},
		{"a variable nothing sets stays", "node ${APP:-x.js}", map[string]string{"APP": "y.js"}, []string{"1 node ${APP:-x.js}"}},
		{"quotes come off", `node "dist/my app.js" 'a b'`, nil, []string{"1 node dist/my app.js a b"}},
		{"a comment is no command", "# tsx src/index.ts\nnode x.js # ts-node\n", nil, []string{"2 node x.js"}},
		{"text that is not shell is kept whole", "node x.js &&& (", nil, []string{"1 node x.js &&& ("}},
		{"nothing", "  \n", nil, nil},
	}
	for _, c := range cases {
		var got []string
		for _, sc := range shellCommands(c.text, c.env) {
			got = append(got, strconv.Itoa(sc.Line)+" "+sc.Text)
		}
		if !reflect.DeepEqual(got, c.want) {
			t.Errorf("%s: %q, want %q", c.name, got, c.want)
		}
	}
}

func TestInstructionLines(t *testing.T) {
	src := "# escape=`\nFROM node:24 AS build\nRUN npm ci `\n  # between the lines\n  && npm run build\nFROM --platform=linux/amd64 node:24-slim\nENV APP=dist/index.js PORT=\"8080\"\nRUN <<EOF\nnpm ci\nCMD [\"not an instruction\"]\nEOF\ncmd [\"node\", \"x.js\"]\n"
	var got []string
	ins := instructionLines([]byte(src))
	for _, in := range ins {
		got = append(got, strconv.Itoa(in.Line)+" "+in.Text)
	}
	want := []string{
		"2 FROM node:24 AS build",
		"3 RUN npm ci && npm run build",
		"6 FROM --platform=linux/amd64 node:24-slim",
		`7 ENV APP=dist/index.js PORT="8080"`,
		`8 RUN <<EOF ; npm ci ; CMD ["not an instruction"]`,
		`12 CMD ["node", "x.js"]`,
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("instructions:\n%s\nwant:\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
	if env := stageEnv(finalStage(ins)); env["APP"] != "dist/index.js" || env["PORT"] != "8080" {
		t.Errorf("env = %v", env)
	}
	if last := ins[len(ins)-1]; !last.Exec || !reflect.DeepEqual(last.Args, []string{"node", "x.js"}) {
		t.Errorf("exec form = %+v", last)
	}
}

// What only a parser reads right: a here-document's lines are no instructions, a printed
// sentence is no command, and a shell-form CMD's variable is the stage's ENV.
func TestStartCommandsReadTheSyntax(t *testing.T) {
	cases := []struct {
		name  string
		files map[string]string
		want  string
	}{
		{"a here-document holding CMD", map[string]string{
			"Dockerfile": "FROM node:24\nRUN <<EOF\nCMD [\"tsx\", \"src/index.ts\"]\nEOF\nCMD [\"node\", \"dist/index.js\"]\n"},
			"Dockerfile:5 node dist/index.js"},
		{"a script that prints what to run", map[string]string{
			"Dockerfile": "FROM node:24\nCMD [\"./start.sh\"]\n",
			"start.sh":   "#!/bin/sh\necho \"run npx prisma migrate deploy by hand\"\nexec node dist/index.js\n"},
			"Dockerfile:2 ./start.sh,start.sh:3 exec node dist/index.js"},
		{"ENV in a shell-form CMD", map[string]string{
			"Dockerfile": "FROM node:24\nENV ENTRY=src/index.ts\nCMD tsx $ENTRY\n"},
			"Dockerfile:3 tsx src/index.ts"},
		{"exec form through sh -c", map[string]string{
			"Dockerfile": "FROM node:24\nCMD [\"sh\", \"-c\", \"npx prisma migrate deploy && node dist/index.js\"]\n"},
			"Dockerfile:2 npx prisma migrate deploy,Dockerfile:2 node dist/index.js"},
	}
	for _, c := range cases {
		var got []string
		for _, sc := range startCommands(memRepo(c.files)) {
			got = append(got, sc.File+":"+strconv.Itoa(sc.Line)+" "+sc.Text)
		}
		if strings.Join(got, ",") != c.want {
			t.Errorf("%s: %q, want %q", c.name, strings.Join(got, ","), c.want)
		}
	}
	printed := memRepo(map[string]string{
		"Dockerfile": "FROM node:24\nCMD [\"./start.sh\"]\n",
		"start.sh":   "#!/bin/sh\necho \"run npx prisma migrate deploy by hand\"\nexec node dist/index.js\n",
	})
	if out := w102(printed, Context{}); len(out.findings) != 0 {
		t.Errorf("a printed sentence was read as a migration: %+v", out.findings)
	}
}
