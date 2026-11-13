package doctor

import "testing"

// W175: a job queue on a cache that evicts loses jobs, so doctor asks for kv_evict: false
// (SKILL.md §9).
func TestW175QueueOnAnEvictingCache(t *testing.T) {
	manifest := passing["whisk.yaml"]
	medusa := `module.exports = defineConfig({ modules: [{ key: Modules.EVENT_BUS, resolve: "@medusajs/medusa/event-bus-redis", options: { redisUrl: kv } }] })` + "\n"
	cases := []struct {
		name  string
		files map[string]string
		want  bool
	}{
		{"bullmq on an evicting cache", map[string]string{"whisk.yaml": manifest + "kv: true\n", "src/jobs.ts": `import { Queue } from "bullmq";` + "\n"}, true},
		{"medusa's event bus", map[string]string{"whisk.yaml": manifest + "kv: true\n", "medusa-config.ts": medusa}, true},
		{"rq", map[string]string{"whisk.yaml": manifest + "kv: true\n", "app/jobs.py": "from rq import Queue\n"}, true},
		{"asynq", map[string]string{"whisk.yaml": manifest + "kv: true\n", "jobs.go": "package main\nimport \"github.com/hibiken/asynq\"\n"}, true},
		{"bullmq on a cache that keeps its keys", map[string]string{"whisk.yaml": manifest + "kv: true\nkv_evict: false\n", "src/jobs.ts": `import { Queue } from "bullmq";` + "\n"}, false},
		{"medusa on a cache that keeps its keys", map[string]string{"whisk.yaml": manifest + "kv: true\nkv_evict: false\n", "medusa-config.ts": medusa}, false},
		{"bullmq without a cache", map[string]string{"src/jobs.ts": `import { Queue } from "bullmq";` + "\n"}, false},
		{"a plain redis client", map[string]string{"whisk.yaml": manifest + "kv: true\n", "src/kv.ts": `import { createClient } from "redis";` + "\n"}, false},
		{"bullmq in browser code", map[string]string{"whisk.yaml": manifest + "kv: true\n", "public/app.js": `import { Queue } from "bullmq";` + "\n"}, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			rep := runDoctor(t, with(passing, c.files), false)
			if got := has(rep, "W175"); got != c.want {
				t.Errorf("W175 = %v, want %v; findings %+v; skipped %v", got, c.want, rep.Findings, rep.Skipped)
			}
		})
	}
}
