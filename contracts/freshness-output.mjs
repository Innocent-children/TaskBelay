import { inputObject, field } from '../scripts/interface-schema.mjs';

const object = (description, properties, optional = []) => ({ description, ...inputObject(properties, optional) });
const text = description => field(description);
const enumeration = (description, values) => field(description, 'string', { enum: values });
const failure = object('Original Git or integration failure, retained for inspection.', {
  message: text('Original failure message.'),
  code: field('Original process exit code or OS error code; null when unavailable.', ['integer', 'string', 'null']),
  stderr: text('Original diagnostic output, bounded by the producer.'),
});

export const freshnessOutputSchema = object('Complete source freshness result, including optional fetch, integration and recovery records.', {
  repositories: field('Actual result for every selected repository.', 'array', {
    items: object('One bound repository and its observed operation result.', {
      key: text('Stable repository key supplied by the caller.'),
      repository_path: text('Canonical absolute worktree path.'),
      source_selection: object('Original selected source, independent of task-branch tracking.', {
        source_type: enumeration('Original source type.', ['local', 'remote']),
        remote_name: text('Selected remote name; empty for a local source.'),
        base_branch: field('Selected source branch; null for an unselected detached source.', ['string', 'null']),
      }),
      remote_freshness: object('Actual local comparison and whether a fetch completed.', {
        head: text('Observed local HEAD.'),
        upstream: text('Selected remote-tracking ref; empty when unavailable.'),
        remote_head: text('Observed fetched commit; empty when unavailable.'),
        state: enumeration('Observed freshness or reason it is unknown.', ['not_checked', 'not_fetched', 'fetch_failed', 'no_upstream', 'detached', 'current', 'ahead', 'behind', 'diverged']),
        fetched: field('Whether this observation includes a successful fetch.', 'boolean'),
        remote_name: text('Actual source remote; empty when unavailable.'),
        remote_ref: text('Actual source branch ref; empty when unavailable.'),
        fetched_at: text('UTC time of the successful fetch; absent when no fetch completed or after read-only recovery.'),
        checked_at: text('UTC time of this observation.'),
        basis: text('Concrete explanation of the observation and its limits.'),
      }, ['fetched_at']),
      action: enumeration('Suggested next action; this does not grant authority.', ['inspect', 'fast_forward_available', 'preserve_dirty', 'preserve_divergence', 'continue', 'preserve_and_inspect']),
      integrated: field('Whether this operation verified a successful fast-forward.', 'boolean'),
      fetch_status: enumeration('Saved fetch attempt state; absent when no fetch was attempted.', ['attempting', 'completed', 'failed', 'unknown']),
      failure,
      integration: object('Per-repository integration outcome and optional recovery observation.', {
        state: enumeration('Saved integration attempt state.', ['pending', 'not_needed', 'blocked', 'applying', 'applied', 'unknown']),
        target_head: text('Frozen fetched target commit; empty before one is observed.'),
        basis: text('Reason for the saved integration result.'),
        attempted_at: text('UTC time saved before the merge attempt.'),
        finished_at: text('UTC time the successful merge was verified.'),
        failure,
        recovery: object('Read-only inspection after the original owner stopped; not proof of the original command result.', {
          checked_at: text('UTC time of recovery inspection.'),
          head: text('Observed local HEAD during recovery.'),
          clean: field('Whether the inspected worktree is clean.', 'boolean'),
          observation: enumeration('Current Git state relative to the saved source and target.', ['target_present', 'original_state_present', 'workspace_drift']),
          basis: text('Limits of the read-only recovery observation.'),
        }),
      }, ['attempted_at', 'finished_at', 'failure', 'recovery']),
    }, ['fetch_status', 'failure']),
  }),
  integration_blocked: field('Whether the integration batch is blocked, partial, uncertain or reconciled rather than complete.', 'boolean'),
  operation: object('Original integration operation; absent for observation-only requests.', {
    operation_id: text('Stable integration identity supplied by the caller.'),
    receipt_path: text('Absolute path of the retained operation receipt.'),
    status: enumeration('Saved batch state.', ['assessing', 'integrating', 'complete', 'blocked', 'partial', 'uncertain', 'reconciled']),
    replayed: field('Whether this response reads an existing operation without repeating effects.', 'boolean'),
    error: text('Original operation failure when present.'),
    receipt_error: text('Failure while saving the operation result when present.'),
  }, ['error', 'receipt_error']),
}, ['operation']);
