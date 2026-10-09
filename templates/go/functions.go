// The app's functions: work that runs on the platform's queue, outside a request, retried step
// by step. Each is declared under functions in whisk.yaml with its graph in workflows/; step
// names match the graph's ids, and doctor checks that they do.
package main

import (
	"context"
	"strings"

	"github.com/inngest/inngestgo"
	"github.com/inngest/inngestgo/step"
)

func registerFunctions(client inngestgo.Client, db *DB) error {
	// note-added runs for every note.added event POST /notes sends. Each step runs once and its
	// result is kept, so a retry resumes after the last finished step; both steps are safe to
	// run twice. A function runs for nobody in particular, so it reads and writes as the system.
	_, err := inngestgo.CreateFunction(client, inngestgo.FunctionOpts{ID: "note-added", Retries: inngestgo.IntPtr(3)}, inngestgo.EventTrigger("note.added", nil),
		func(ctx context.Context, in inngestgo.Input[map[string]any]) (any, error) {
			raw, _ := in.Event.Data["note_id"].(float64)
			id := int64(raw)
			words, err := step.Run(ctx, "count-words", func(ctx context.Context) (int, error) {
				note, ok, err := db.GetNote(ctx, systemCaller, id)
				if err != nil || !ok {
					return -1, err
				}
				return len(strings.Fields(note.Body)), nil
			})
			if err != nil {
				return nil, err
			}
			if words < 0 {
				return map[string]any{"note_id": id, "found": false}, nil
			}
			if _, err := step.Run(ctx, "save-count", func(ctx context.Context) (bool, error) { return true, db.SetWords(ctx, systemCaller, id, words) }); err != nil {
				return nil, err
			}
			return map[string]any{"note_id": id, "words": words}, nil
		})
	return err
}
