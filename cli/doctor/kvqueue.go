package doctor

import (
	"fmt"
	"regexp"

	"github.com/whisk-run/cli/internal/stack"
)

// A job queue on the app's cache loses jobs when the cache evicts keys to make room, which it
// does unless whisk.yaml sets kv_evict: false (SKILL.md §9). W175 finds the queue libraries by
// the import or module name that loads them.

// kvQueues are the job queue libraries that keep their jobs in Redis, by the import that loads
// them; Medusa's Redis event bus and workflow engine are named as modules in medusa-config.
var kvQueues = map[stack.Lang][]*regexp.Regexp{
	stack.JS: {
		regexp.MustCompile(`(?:from\s+|require\(\s*|import\s+)["'](bullmq|bull|bee-queue)["']`),
		regexp.MustCompile(`["'](@medusajs/(?:medusa/)?(?:event-bus-redis|workflow-engine-redis))["']`),
	},
	stack.PY: {regexp.MustCompile(`(?m)^\s*(?:from|import)\s+(rq|arq|celery|dramatiq)\b`)},
	stack.GO: {regexp.MustCompile(`"(github\.com/hibiken/asynq)"`)},
}

func w175(r Repo, _ Context) outcome {
	if !r.Manifest.KV || r.Manifest.KeepsKVKeys() {
		return outcome{}
	}
	for _, f := range serverCode(r) {
		if hs := matches(f, kvQueues[stack.LangOf(f.Path)], 1); len(hs) > 0 {
			return one("W175", f.Path, hs[0].Line, fmt.Sprintf("%s runs a job queue on the cache with %s, and the cache drops keys when it is full, which loses jobs; set kv_evict: false in whisk.yaml.", f.Path, hs[0].Name))
		}
	}
	return outcome{}
}
