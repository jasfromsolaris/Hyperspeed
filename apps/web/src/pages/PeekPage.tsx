import { useQueries, useQuery } from "@tanstack/react-query";
import { ExternalLink, Search, X } from "lucide-react";
import { useEffect, useMemo, useState } from "react";
import { Link } from "react-router-dom";
import { apiFetch } from "../api/http";
import { fetchOrganizationsList } from "../api/orgs";
import type {
  OrgMemberWithUser,
  PeekAIActivityEntry,
  PeekAIRunDetail,
  PeekAIRunDetailResponse,
} from "../api/types";
function formatRelativeAgo(iso: string | null | undefined): string {
  if (!iso) return "—";
  const t = new Date(iso).getTime();
  const sec = Math.floor((Date.now() - t) / 1000);
  if (sec < 5) return "just now";
  if (sec < 60) return `${sec}s ago`;
  const m = Math.floor(sec / 60);
  if (m < 60) return `${m}m ago`;
  const h = Math.floor(m / 60);
  if (h < 24) return `${h}h ago`;
  const d = Math.floor(h / 24);
  return `${d}d ago`;
}

function providerLabel(p: string | null | undefined): string {
  const x = (p ?? "").toLowerCase();
  if (x === "cursor") return "Cursor";
  if (x === "openrouter") return "OpenRouter";
  return p ?? "AI";
}

/** Flatten run_detail into monospace-friendly log lines (no inner boxes). */
function linesFromRunDetail(detail: PeekAIRunDetail | null | undefined): string[] {
  if (detail == null || typeof detail !== "object") return [];
  const out: string[] = [];
  const provider = String(detail.provider ?? "");

  if (provider === "openrouter") {
    const trace = detail.trace as { steps?: Record<string, unknown>[] } | undefined;
    const steps = trace?.steps;
    if (Array.isArray(steps)) {
      for (const step of steps) {
        const s = step as Record<string, unknown>;
        const iter = typeof s.iteration === "number" ? s.iteration : "?";
        if (s.reasoning != null || s.reasoning_details != null) {
          const r =
            typeof s.reasoning === "string"
              ? s.reasoning
              : JSON.stringify(s.reasoning ?? s.reasoning_details);
          const one = r.replace(/\s+/g, " ").trim();
          if (one.length > 160) out.push(`[round ${iter}] … ${one.slice(0, 160)}…`);
          else if (one) out.push(`[round ${iter}] ${one}`);
        }
        const at = typeof s.assistant_text === "string" ? s.assistant_text.trim() : "";
        if (at) {
          const t = at.replace(/\s+/g, " ");
          out.push(
            t.length > 200 ? `[round ${iter}] ${t.slice(0, 200)}…` : `[round ${iter}] ${t}`,
          );
        }
        const toolCalls = s.tool_calls as
          | { name?: string; arguments?: unknown }[]
          | undefined;
        const toolResults = s.tool_results as
          | { name?: string; output?: string; truncated?: boolean }[]
          | undefined;
        if (Array.isArray(toolCalls)) {
          toolCalls.forEach((tc, j) => {
            const tag = (tc.name ?? "tool").replace(/\./g, "_").toUpperCase();
            out.push(`✓ ${tag}  COMPLETED`);
            const res = Array.isArray(toolResults) ? toolResults[j] : undefined;
            if (res?.output) {
              let line = res.output.replace(/\s+/g, " ").trim();
              if (line.length > 220) line = line.slice(0, 220) + "…";
              out.push(`  ${line}${res.truncated ? " [truncated]" : ""}`);
            }
          });
        }
      }
    }
    const props = detail.file_edit_proposals as { file_name?: string }[] | undefined;
    if (Array.isArray(props) && props.length > 0) {
      out.push(`— file proposals: ${props.map((p) => p.file_name ?? "?").join(", ")}`);
    }
    if (typeof detail.fallback_note === "string" && detail.fallback_note) {
      out.push(`! fallback: ${detail.fallback_note}`);
    }
  } else if (provider === "cursor") {
    const conv = detail.conversation as { role?: string; content?: string }[] | undefined;
    if (Array.isArray(conv)) {
      for (const m of conv) {
        const role = (m.role ?? "?").toUpperCase();
        const c = (m.content ?? "").replace(/\s+/g, " ").trim();
        if (c.length > 240) out.push(`${role}: ${c.slice(0, 240)}…`);
        else if (c) out.push(`${role}: ${c}`);
      }
    }
  } else if (Object.keys(detail).length > 0) {
    out.push(JSON.stringify(detail, null, 0).slice(0, 400));
  }
  return out;
}

function buildStdOutLines(
  entries: PeekAIActivityEntry[],
  runDetail: PeekAIRunDetail | null | undefined,
): string[] {
  const lines: string[] = [];
  if (entries.length === 0) {
    lines.push("$ idle — no @mention runs in co-visible spaces yet.");
    return lines;
  }

  const [latest, ...rest] = entries;
  const detailLines = linesFromRunDetail(runDetail);
  if (detailLines.length > 0) {
    lines.push("— latest run —");
    lines.push(...detailLines);
  } else {
    const st = latest.responded_at ? "DONE" : "RUNNING";
    lines.push(
      `[${new Date(latest.created_at).toLocaleString()}] ${latest.space_name} · #${latest.chat_room_name} · ${st}`,
    );
    lines.push("(detailed tool trace unavailable for this run)");
  }

  for (const e of rest.slice(0, 4)) {
    const st = e.responded_at ? "DONE" : "RUNNING";
    lines.push(
      `[${new Date(e.created_at).toLocaleString()}] ${e.space_name} · #${e.chat_room_name} · ${st}`,
    );
  }
  if (rest.length > 4) {
    lines.push(`… +${rest.length - 4} older runs`);
  }
  return lines;
}

type DetailQueryLike = {
  isPending: boolean;
  isError: boolean;
};

function PeekAgentSurface({
  variant,
  activeGlowClass,
  label,
  staff,
  hasActive,
  entries,
  latest,
  lastTouch,
  stdLines,
  detailQ,
  chatHref,
  onExpand,
  onClose,
}: {
  variant: "card" | "modal";
  activeGlowClass?: string;
  label: string;
  staff: OrgMemberWithUser;
  hasActive: boolean;
  entries: PeekAIActivityEntry[];
  latest: PeekAIActivityEntry | undefined;
  lastTouch: string | null;
  stdLines: string[];
  detailQ: DetailQueryLike;
  chatHref: string | null;
  onExpand?: () => void;
  onClose?: () => void;
}) {
  const terminalMax =
    variant === "modal"
      ? "max-h-[min(70vh,720px)] min-h-[16rem]"
      : "max-h-52 min-h-[9rem]";
  const logText = variant === "modal" ? "text-xs" : "text-[10px]";
  const headerTitle =
    variant === "modal" ? "text-lg font-semibold" : "font-semibold leading-tight";

  const body = (
    <>
      <div
        className={
          variant === "modal"
            ? "border-b border-border/60 bg-muted/20 px-4 py-4"
            : "border-b border-border/60 bg-muted/20 px-3 py-2.5"
        }
      >
        {variant === "card" ? (
          <div className="flex items-start gap-2.5">
            <span
              className={[
                "mt-1.5 h-2 w-2 shrink-0 rounded-full",
                hasActive
                  ? "bg-emerald-400 shadow-[0_0_10px_2px_rgba(52,211,153,0.7)]"
                  : "bg-muted-foreground/35",
              ].join(" ")}
              aria-hidden
            />
            <div className="min-w-0 flex-1">
              <div className="flex items-start justify-between gap-2">
                <div className={["truncate text-foreground", headerTitle].join(" ")}>
                  {label}
                </div>
                {onExpand ? (
                  <button
                    type="button"
                    className="shrink-0 rounded-md p-1 text-muted-foreground transition-colors hover:bg-muted hover:text-primary"
                    title="Focus view"
                    aria-label="Open focus view"
                    onClick={onExpand}
                  >
                    <ExternalLink className="h-3.5 w-3.5" />
                  </button>
                ) : null}
              </div>
              <p className="mt-0.5 text-[11px] text-muted-foreground">
                {hasActive ? (
                  <span className="text-emerald-600 dark:text-emerald-400">Live now</span>
                ) : entries.length === 0 ? (
                  "Idle"
                ) : (
                  <>Finished {formatRelativeAgo(lastTouch)}</>
                )}
              </p>
              <p className="mt-1 text-[10px] text-muted-foreground/90">
                {providerLabel(staff.service_account_provider)}
                {staff.openrouter_model ? ` · ${staff.openrouter_model}` : ""}
              </p>
            </div>
          </div>
        ) : (
          <div className="flex items-start justify-between gap-4">
            <div className="min-w-0 flex-1">
              <h2
                id="peek-focus-title"
                className="text-lg font-semibold tracking-tight text-foreground"
              >
                {label}
              </h2>
              <p className="mt-1 text-sm text-muted-foreground">
                {hasActive ? (
                  <span className="text-emerald-600 dark:text-emerald-400">Live now</span>
                ) : entries.length === 0 ? (
                  "Idle"
                ) : (
                  <>Finished {formatRelativeAgo(lastTouch)}</>
                )}
              </p>
              <p className="mt-2 text-xs text-muted-foreground">
                {providerLabel(staff.service_account_provider)}
                {staff.openrouter_model ? ` · ${staff.openrouter_model}` : ""}
              </p>
            </div>
            <div className="flex shrink-0 items-center gap-2">
              {chatHref ? (
                <Link
                  to={chatHref}
                  className="text-sm font-medium text-primary hover:underline"
                >
                  Open chat
                </Link>
              ) : null}
              {onClose ? (
                <button
                  type="button"
                  className="rounded-md p-2 text-muted-foreground transition-colors hover:bg-muted hover:text-foreground"
                  aria-label="Close focus view"
                  onClick={onClose}
                >
                  <X className="h-5 w-5" />
                </button>
              ) : null}
            </div>
          </div>
        )}
      </div>

      {latest ? (
        <div
          className={
            variant === "modal"
              ? "border-b border-border/40 bg-background/40 px-4 py-3"
              : "border-b border-border/40 bg-background/40 px-3 py-2"
          }
        >
          <p className="text-[10px] font-medium uppercase tracking-[0.15em] text-muted-foreground/80">
            Current focus
          </p>
          <p
            className={[
              "mt-1 font-mono leading-snug text-teal-600/95 dark:text-teal-400/95",
              variant === "modal" ? "text-sm" : "text-[11px]",
            ].join(" ")}
          >
            {latest.responded_at == null ? "● " : ""}
            {latest.space_name} · #{latest.chat_room_name}
            {latest.responded_at == null ? " · RUNNING" : " · done"}
          </p>
        </div>
      ) : (
        <div
          className={
            variant === "modal"
              ? "border-b border-border/40 px-4 py-3 text-sm text-muted-foreground"
              : "border-b border-border/40 px-3 py-2 text-[11px] text-muted-foreground"
          }
        >
          No runs yet
        </div>
      )}

      <div className="flex min-h-0 flex-1 flex-col bg-[#0c0c0e] dark:bg-black/40">
        <div className="flex items-center gap-1.5 border-b border-white/5 px-2 py-1.5 font-mono text-[9px] font-semibold uppercase tracking-[0.2em] text-muted-foreground/70">
          <span className="text-emerald-500/90">$</span>
          <span>stdout</span>
          <span className="text-white/25">&gt;</span>
        </div>
        <div
          className={[
            "overflow-y-auto px-2.5 py-2 font-mono leading-relaxed text-white/85",
            terminalMax,
            logText,
          ].join(" ")}
        >
          {detailQ.isPending && latest ? (
            <p className="animate-pulse text-white/45">streaming…</p>
          ) : detailQ.isError ? (
            <p className="text-rose-400/90">could not load run log</p>
          ) : (
            stdLines.map((line, i) => (
              <div
                key={i}
                className={[
                  "whitespace-pre-wrap break-words",
                  line.startsWith("✓") ? "text-emerald-400" : "text-white/85",
                ].join(" ")}
              >
                {line}
              </div>
            ))
          )}
          <div className="mt-2 border-t border-white/5 pt-1.5 text-[9px] text-white/35">
            stdout &gt;
          </div>
        </div>
      </div>
    </>
  );

  if (variant === "card") {
    return (
      <section
        className={[
          "flex min-h-[14rem] flex-col overflow-hidden rounded-xl border bg-card",
          activeGlowClass ?? "",
        ].join(" ")}
      >
        {body}
      </section>
    );
  }

  return body;
}

export default function PeekPage() {
  const [focusIdx, setFocusIdx] = useState<number | null>(null);

  const orgsQ = useQuery({
    queryKey: ["orgs"],
    queryFn: fetchOrganizationsList,
  });

  const orgId = orgsQ.data?.organizations?.[0]?.id ?? "";

  const peekQ = useQuery({
    queryKey: ["peek-ai-activity", orgId],
    enabled: !!orgId,
    queryFn: async () => {
      const res = await apiFetch(
        `/api/v1/organizations/${orgId}/peek/ai-activity?limit=200`,
      );
      if (!res.ok) throw new Error("peek");
      return res.json() as Promise<{
        activities: PeekAIActivityEntry[];
        co_visible_ai_user_ids: string[];
      }>;
    },
  });

  const membersQ = useQuery({
    queryKey: ["org-members", orgId],
    enabled: !!orgId,
    queryFn: async () => {
      const res = await apiFetch(`/api/v1/organizations/${orgId}/members`);
      if (!res.ok) throw new Error("members");
      const j = (await res.json()) as { members: OrgMemberWithUser[] };
      return j.members ?? [];
    },
  });

  const coSet = useMemo(() => {
    const ids = peekQ.data?.co_visible_ai_user_ids ?? [];
    return new Set(ids);
  }, [peekQ.data?.co_visible_ai_user_ids]);

  const staffCards = useMemo(() => {
    const members = membersQ.data ?? [];
    const out: OrgMemberWithUser[] = [];
    for (const m of members) {
      if (!m.is_service_account) continue;
      if (!coSet.has(m.user_id)) continue;
      out.push(m);
    }
    out.sort((a, b) => {
      const la =
        a.display_name?.trim() || a.email.split("@")[0] || a.email;
      const lb =
        b.display_name?.trim() || b.email.split("@")[0] || b.email;
      return la.localeCompare(lb, undefined, { sensitivity: "base" });
    });
    return out;
  }, [membersQ.data, coSet]);

  const byAI = useMemo(() => {
    const m = new Map<string, PeekAIActivityEntry[]>();
    for (const a of peekQ.data?.activities ?? []) {
      const list = m.get(a.ai_user_id) ?? [];
      list.push(a);
      m.set(a.ai_user_id, list);
    }
    for (const [, list] of m) {
      list.sort(
        (x, y) =>
          new Date(y.created_at).getTime() - new Date(x.created_at).getTime(),
      );
    }
    return m;
  }, [peekQ.data?.activities]);

  const latestDetailQueries = useQueries({
    queries: staffCards.map((staff) => {
      const first = byAI.get(staff.user_id)?.[0];
      return {
        queryKey: ["peek-ai-run-detail", orgId, first?.id ?? ""],
        enabled: !!orgId && !!first?.id,
        queryFn: async () => {
          const id = first?.id;
          if (!id) throw new Error("no run");
          const res = await apiFetch(
            `/api/v1/organizations/${orgId}/peek/ai-activity/runs/${id}`,
          );
          if (!res.ok) throw new Error("peek run");
          return res.json() as Promise<PeekAIRunDetailResponse>;
        },
      };
    }),
  });

  const loading = orgsQ.isLoading || !orgId || peekQ.isPending || membersQ.isPending;

  useEffect(() => {
    if (focusIdx === null) return;
    const prevOverflow = document.body.style.overflow;
    document.body.style.overflow = "hidden";
    const onKey = (e: KeyboardEvent) => {
      if (e.key === "Escape") setFocusIdx(null);
    };
    window.addEventListener("keydown", onKey);
    return () => {
      document.body.style.overflow = prevOverflow;
      window.removeEventListener("keydown", onKey);
    };
  }, [focusIdx]);

  const focusStaff =
    focusIdx !== null ? staffCards[focusIdx] : undefined;
  const focusEntries = focusStaff
    ? (byAI.get(focusStaff.user_id) ?? [])
    : [];
  const focusLatest = focusEntries[0];
  const focusLastTouch =
    focusLatest?.responded_at ?? focusLatest?.created_at ?? null;
  const focusDetailQ =
    focusIdx !== null ? latestDetailQueries[focusIdx] : undefined;
  const focusRunDetail = focusDetailQ?.data?.reply?.run_detail as
    | PeekAIRunDetail
    | undefined;
  const focusStdLines = buildStdOutLines(
    focusEntries,
    focusRunDetail ?? null,
  );
  const focusChatHref = focusLatest
    ? `/o/${orgId}/p/${focusLatest.space_id}/c/${focusLatest.chat_room_id}`
    : null;
  const focusHasActive = focusEntries.some((e) => e.responded_at == null);

  return (
    <>
    <div className="min-h-0 flex-1 overflow-y-auto bg-background px-4 py-8 md:px-8">
      <div className="mx-auto max-w-2xl">
        <header className="mb-8 flex flex-col gap-2 sm:flex-row sm:items-end sm:justify-between">
          <div className="flex items-center gap-2">
            <span className="flex h-9 w-9 items-center justify-center rounded-md border border-border bg-card text-muted-foreground">
              <Search className="h-4 w-4" aria-hidden />
            </span>
            <div>
              <h1 className="text-2xl font-semibold tracking-tight text-foreground">
                Peek
              </h1>
              <p className="mt-0.5 text-sm text-muted-foreground">
                Live stream of AI staff reasoning and tool use from @mentions.
              </p>
            </div>
          </div>
        </header>

        {loading ? (
          <p className="text-sm text-muted-foreground">Loading…</p>
        ) : !orgId ? (
          <p className="text-sm text-muted-foreground">No workspace available.</p>
        ) : staffCards.length === 0 ? (
          <div className="rounded-lg border border-dashed border-border bg-muted/20 px-4 py-8 text-center text-sm text-muted-foreground">
            No AI staff co-visible with you in this workspace yet, or none share a space
            with you.
          </div>
        ) : (
          <div className="grid grid-cols-1 gap-4 md:grid-cols-2">
            {staffCards.map((staff, idx) => {
              const entries = byAI.get(staff.user_id) ?? [];
              const label =
                staff.display_name?.trim() ||
                staff.email.split("@")[0] ||
                staff.email;
              const hasActive = entries.some((e) => e.responded_at == null);
              const latest = entries[0];
              const lastTouch =
                latest?.responded_at ?? latest?.created_at ?? null;
              const detailQ = latestDetailQueries[idx];
              const runDetail = detailQ.data?.reply?.run_detail as
                | PeekAIRunDetail
                | undefined;
              const stdLines = buildStdOutLines(entries, runDetail ?? null);

              const activeGlow = hasActive
                ? "border-emerald-500/50 shadow-[0_0_24px_-6px_rgba(16,185,129,0.55)] ring-1 ring-emerald-500/25"
                : "border-border/70 shadow-sm ring-0";

              return (
                <PeekAgentSurface
                  key={staff.user_id}
                  variant="card"
                  activeGlowClass={activeGlow}
                  label={label}
                  staff={staff}
                  hasActive={hasActive}
                  entries={entries}
                  latest={latest}
                  lastTouch={lastTouch}
                  stdLines={stdLines}
                  detailQ={detailQ}
                  chatHref={null}
                  onExpand={() => setFocusIdx(idx)}
                />
              );
            })}
          </div>
        )}
      </div>
    </div>
    {focusIdx !== null && focusStaff ? (
      <div
        className="fixed inset-0 z-50 flex items-center justify-center p-4"
        role="dialog"
        aria-modal="true"
        aria-labelledby="peek-focus-title"
      >
        <button
          type="button"
          className="absolute inset-0 bg-black/60 backdrop-blur-[1px]"
          aria-label="Close focus view"
          onClick={() => setFocusIdx(null)}
        />
        <div className="relative z-10 flex max-h-[90vh] w-full max-w-4xl flex-col overflow-hidden rounded-xl border border-border bg-card shadow-2xl">
          <PeekAgentSurface
            variant="modal"
            label={
              focusStaff.display_name?.trim() ||
              focusStaff.email.split("@")[0] ||
              focusStaff.email
            }
            staff={focusStaff}
            hasActive={focusHasActive}
            entries={focusEntries}
            latest={focusLatest}
            lastTouch={focusLastTouch}
            stdLines={focusStdLines}
            detailQ={focusDetailQ ?? { isPending: false, isError: false }}
            chatHref={focusChatHref}
            onClose={() => setFocusIdx(null)}
          />
        </div>
      </div>
    ) : null}
    </>
  );
}
