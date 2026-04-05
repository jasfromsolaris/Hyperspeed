# AI Agent Release Readiness Checklist

## Go / No-Go Criteria

- [ ] Frontend build passes (`apps/web`).
- [ ] Backend compile/test passes (`apps/api`).
- [ ] Mode policy tests pass.
- [ ] QA matrix critical paths pass.
- [ ] Audit rows include outcome category for failures.
- [ ] MCP auth failures are actionable.

## Pre-Release Validation

1. Verify Ask/Plan/Agent mode selector behavior in IDE right panel.
2. Verify folder-first guardrails block invocations before project folder selection.
3. Verify proposal flow from Agent mode and proposal review acceptance path.
4. Verify human-only direct apply remains confirm-gated.
5. Verify service accounts never receive direct apply UI/action.
6. Smoke OpenRouter staff mentions with tools enabled (see `ai-agent-qa-matrix.md` OpenRouter section); confirm `OPENROUTER_CHAT_TOOLS_ENABLED=false` fallback if needed.

## Observability

1. Inspect latest `agent_tool_invocations` rows for:
   - `tool`
   - `session_id`
   - `duration_ms`
   - `error_text` prefixed with taxonomy category (for failures)
2. Confirm mode policy denials are visible and searchable.

## Rollback Plan

If severe regressions occur:

1. Disable direct-apply usage in UI (proposal-only fallback).
2. Revert MCP clients to `mode=agent` default only.
3. Roll back API changes to previous invoke behavior.
4. Re-run smoke tests on stable commit before re-enabling.

## Post-Release Monitoring Window

- First 24h: monitor invoke error rate, mode-policy denials, and auth failures.
- First 72h: sample transcripts/logs for user confusion around mode semantics.
