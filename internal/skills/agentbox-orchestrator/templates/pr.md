# <conventional commit title>

<!-- orchestrator-workflow: {"workflow":"<workflow-id>","issue":<issue>,"stage":"implement","worker":"<runtime>","worker_id":"<worker-id>","model":"<model-id>","commit":"<full-sha>","pr":<pr>} -->

## Summary

What changed and why, in two or three sentences that do not assume the reader
followed the issue.

## Changes

- The behavior that changed, grouped by area.

## Evidence

- `<command>` — result, run at `<commit>`.
- Full gates: pending until the independent verification worker reports them.

## Closes #<issue>

## Risks

What could regress, what remains untested, and who should know.

## Provenance

See the workflow marker above. This body is authored by the orchestrator on
behalf of the named stage, worker, and model; it does not imply a human wrote
it.

Orchestrated by agentbox-orchestrator {{VERSION}} · workflow <workflow-id> · stage implement · worker <runtime>/<agent> · model <model-id> · commit <full-sha>
