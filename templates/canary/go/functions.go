// Three functions, one per trigger kind the platform offers. Step names match the ids in
// workflows/*.graph.yaml; doctor checks that they do.
package main

import (
	"context"
	"errors"
	"time"

	"github.com/inngest/inngestgo"
	"github.com/inngest/inngestgo/step"
)

func registerFunctions(client inngestgo.Client, db *DB) error {
	_, err := inngestgo.CreateFunction(client, inngestgo.FunctionOpts{ID: "nightly-summary"}, inngestgo.CronTrigger("0 6 * * *"),
		func(ctx context.Context, in inngestgo.Input[any]) (any, error) {
			total, err := step.Run(ctx, "count-notes", func(ctx context.Context) (int, error) { return db.CountNotes(ctx) })
			if err != nil {
				return nil, err
			}
			day := time.Now().UTC().Format("2006-01-02")
			return step.Run(ctx, "record-summary", func(ctx context.Context) (Recorded, error) {
				return db.RecordEvent(ctx, "summary:"+day, "summary", map[string]any{"day": day, "notes": total}, false)
			})
		})
	if err != nil {
		return err
	}

	_, err = inngestgo.CreateFunction(client, inngestgo.FunctionOpts{ID: "canary-event", Retries: inngestgo.IntPtr(3)}, inngestgo.EventTrigger("canary.event", nil),
		func(ctx context.Context, in inngestgo.Input[map[string]any]) (any, error) {
			data, err := step.Run(ctx, "receive", func(ctx context.Context) (map[string]any, error) {
				out := map[string]any{"run_id": in.InputCtx.RunID}
				for k, v := range in.Event.Data {
					out[k] = v
				}
				return out, nil
			})
			if err != nil {
				return nil, err
			}
			result, err := step.Run(ctx, "compute", func(ctx context.Context) (map[string]any, error) {
				if fail, _ := data["fail"].(bool); fail {
					return nil, errors.New("canary.event asked to fail")
				}
				sum := 0.0
				if numbers, ok := data["numbers"].([]any); ok {
					for _, n := range numbers {
						if f, ok := n.(float64); ok {
							sum += f
						}
					}
				}
				data["sum"] = sum
				return data, nil
			})
			if err != nil {
				return nil, err
			}
			return step.Run(ctx, "record", func(ctx context.Context) (Recorded, error) {
				return db.RecordEvent(ctx, in.InputCtx.RunID, "run", result, false)
			})
		})
	if err != nil {
		return err
	}

	_, err = inngestgo.CreateFunction(client, inngestgo.FunctionOpts{ID: "canary-approval"}, inngestgo.EventTrigger("canary.approval", nil),
		func(ctx context.Context, in inngestgo.Input[map[string]any]) (any, error) {
			runID := in.InputCtx.RunID
			decision, err := approval(ctx, runID, "request-approval", ApprovalRequest{To: "owner", Title: "Canary approval for run " + runID, Data: in.Event.Data})
			if err != nil {
				return nil, err
			}
			if decision.Decision == "approved" {
				return step.Run(ctx, "record-decision", func(ctx context.Context) (Recorded, error) {
					payload := map[string]any{"approval_id": decision.ApprovalID, "decision": decision.Decision, "actor": decision.Actor, "at": decision.At, "note": decision.Note}
					for k, v := range in.Event.Data {
						payload[k] = v
					}
					return db.RecordEvent(ctx, runID+":decision", "approval", payload, false)
				})
			}
			return step.Run(ctx, "notify-rejection", func(ctx context.Context) (string, error) {
				logger.Info("approval not granted", "run_id", runID, "decision", decision.Decision)
				return decision.Decision, nil
			})
		})
	return err
}
