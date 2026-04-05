import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { MonacoDiffViewer } from "./MonacoDiffViewer";
import { ChevronDown, ChevronUp, FileCode2 } from "lucide-react";
import { useMemo, useState } from "react";
import { apiFetch } from "../api/http";
import type { FileEditProposal, UUID } from "../api/types";

function monacoLanguageFromFileName(name: string): string {
  const ext = name.split(".").pop()?.toLowerCase() ?? "";
  switch (ext) {
    case "ts":
      return "typescript";
    case "tsx":
      return "typescript";
    case "js":
      return "javascript";
    case "jsx":
      return "javascript";
    case "go":
      return "go";
    case "md":
      return "markdown";
    case "json":
      return "json";
    case "css":
      return "css";
    case "html":
      return "html";
    case "yaml":
    case "yml":
      return "yaml";
    default:
      return "plaintext";
  }
}

/** Rough +/− line counts for the diff header (best-effort, line-based). */
function diffLineStats(original: string, modified: string): { add: number; del: number } {
  const a = original.split("\n");
  const b = modified.split("\n");
  let i = 0;
  let j = 0;
  let add = 0;
  let del = 0;
  while (i < a.length || j < b.length) {
    if (i >= a.length) {
      add += b.length - j;
      break;
    }
    if (j >= b.length) {
      del += a.length - i;
      break;
    }
    if (a[i] === b[j]) {
      i++;
      j++;
      continue;
    }
    const nextMatchA = b.indexOf(a[i], j);
    const nextMatchB = a.indexOf(b[j], i);
    if (nextMatchA !== -1 && (nextMatchB === -1 || nextMatchA - j <= nextMatchB - i)) {
      add += nextMatchA - j;
      j = nextMatchA;
      continue;
    }
    if (nextMatchB !== -1) {
      del += nextMatchB - i;
      i = nextMatchB;
      continue;
    }
    del++;
    add++;
    i++;
    j++;
  }
  return { add, del };
}

export function ChatFileProposalCard(props: {
  orgId: string;
  spaceId: string;
  chatRoomId: string;
  proposalId: UUID;
  nodeId: UUID;
  fileNameFallback: string;
}) {
  const { orgId, spaceId, chatRoomId, proposalId, nodeId, fileNameFallback } = props;
  const qc = useQueryClient();
  const [expanded, setExpanded] = useState(true);

  const proposalsQ = useQuery({
    queryKey: ["file-proposals", spaceId, nodeId],
    enabled: !!orgId && !!spaceId && !!nodeId,
    queryFn: async () => {
      const res = await apiFetch(
        `/api/v1/organizations/${orgId}/spaces/${spaceId}/files/${nodeId}/proposals`,
      );
      if (!res.ok) throw new Error("proposals");
      const j = (await res.json()) as { proposals: FileEditProposal[] };
      return j.proposals;
    },
  });

  const fileTextQ = useQuery({
    queryKey: ["file-text", spaceId, nodeId],
    enabled: !!orgId && !!spaceId && !!nodeId && expanded,
    queryFn: async () => {
      const res = await apiFetch(
        `/api/v1/organizations/${orgId}/spaces/${spaceId}/files/${nodeId}/text`,
      );
      if (!res.ok) throw new Error("file-text");
      return res.json() as Promise<{ node: { name: string }; content: string }>;
    },
  });

  const proposal = useMemo(() => {
    return (proposalsQ.data ?? []).find((p) => p.id === proposalId) ?? null;
  }, [proposalsQ.data, proposalId]);

  const displayName = fileTextQ.data?.node?.name?.trim() || fileNameFallback || "File";

  const diffOriginal = useMemo(() => {
    if (!proposal) return "";
    if (proposal.base_content != null) return proposal.base_content;
    return fileTextQ.data?.content ?? "";
  }, [proposal, fileTextQ.data?.content]);

  const stats = useMemo(() => {
    if (!proposal) return { add: 0, del: 0 };
    return diffLineStats(diffOriginal, proposal.proposed_content);
  }, [proposal, diffOriginal]);

  const acceptProposal = useMutation({
    mutationFn: async () => {
      const res = await apiFetch(
        `/api/v1/organizations/${orgId}/spaces/${spaceId}/files/proposals/${proposalId}/accept`,
        { method: "POST" },
      );
      if (res.status === 409) {
        const err = await res.json().catch(() => ({}));
        throw new Error((err as { error?: string }).error || "Base file changed");
      }
      if (!res.ok) throw new Error("accept");
    },
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ["file-text", spaceId, nodeId] });
      void qc.invalidateQueries({ queryKey: ["file-proposals", spaceId, nodeId] });
      void qc.invalidateQueries({ queryKey: ["chat-messages", chatRoomId] });
      void qc.invalidateQueries({ queryKey: ["file-nodes", spaceId] });
    },
  });

  const rejectProposal = useMutation({
    mutationFn: async () => {
      const res = await apiFetch(
        `/api/v1/organizations/${orgId}/spaces/${spaceId}/files/proposals/${proposalId}/reject`,
        { method: "POST" },
      );
      if (!res.ok) throw new Error("reject");
    },
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ["file-proposals", spaceId, nodeId] });
      void qc.invalidateQueries({ queryKey: ["chat-messages", chatRoomId] });
    },
  });

  const pending = proposal?.status === "pending";
  const statusLabel =
    proposal?.status === "accepted"
      ? "Accepted"
      : proposal?.status === "rejected"
        ? "Rejected"
        : "Pending review";

  return (
    <div className="mt-2 overflow-hidden rounded-md border border-border bg-card text-left shadow-sm">
      <button
        type="button"
        className="flex w-full items-center gap-2 border-b border-border bg-muted/40 px-3 py-2 text-left text-sm hover:bg-muted/60"
        onClick={() => setExpanded((e) => !e)}
      >
        <FileCode2 className="h-4 w-4 shrink-0 text-sky-500" aria-hidden />
        <span className="min-w-0 flex-1 truncate font-medium text-foreground">{displayName}</span>
        {pending && fileTextQ.data ? (
          <span className="shrink-0 font-mono text-xs">
            <span className="text-emerald-600">+{stats.add}</span>
            <span className="text-muted-foreground"> </span>
            <span className="text-red-600">−{stats.del}</span>
          </span>
        ) : null}
        <span
          className={[
            "shrink-0 rounded-sm px-1.5 py-0.5 text-[11px] font-medium uppercase tracking-wide",
            pending
              ? "bg-amber-500/15 text-amber-700 dark:text-amber-400"
              : proposal?.status === "accepted"
                ? "bg-emerald-500/15 text-emerald-700 dark:text-emerald-400"
                : "bg-muted text-muted-foreground",
          ].join(" ")}
        >
          {statusLabel}
        </span>
        {expanded ? (
          <ChevronUp className="h-4 w-4 shrink-0 text-muted-foreground" aria-hidden />
        ) : (
          <ChevronDown className="h-4 w-4 shrink-0 text-muted-foreground" aria-hidden />
        )}
      </button>

      {expanded && proposal ? (
        <div className="border-b border-border">
          {fileTextQ.isPending ? (
            <div className="flex h-[200px] items-center justify-center text-xs text-muted-foreground">
              Loading diff…
            </div>
          ) : fileTextQ.isError ? (
            <div className="px-3 py-2 text-xs text-red-600">Could not load the original file for this diff.</div>
          ) : (
            <MonacoDiffViewer
              instanceKey={`chat-proposal-${proposalId}`}
              height="min(280px, 40vh)"
              language={monacoLanguageFromFileName(displayName)}
              theme="vs-dark"
              original={diffOriginal}
              modified={proposal.proposed_content}
              options={{
                readOnly: true,
                minimap: { enabled: false },
                renderSideBySide: true,
                enableSplitViewResizing: false,
              }}
              loading={
                <div className="flex h-[200px] items-center justify-center text-xs text-muted-foreground">
                  Preparing editor…
                </div>
              }
            />
          )}
        </div>
      ) : null}

      {expanded && proposalsQ.isError ? (
        <div className="px-3 py-2 text-xs text-red-600">Could not load proposal.</div>
      ) : null}

      {expanded && !proposalsQ.isPending && !proposal ? (
        <div className="px-3 py-2 text-xs text-muted-foreground">
          Proposal not found (it may have been removed).
        </div>
      ) : null}

      {expanded && pending && proposal ? (
        <div className="flex flex-wrap gap-2 px-3 py-2">
          <button
            type="button"
            className="rounded-sm bg-primary px-3 py-1.5 text-sm font-medium text-primary-foreground hover:bg-primary/90 disabled:opacity-50"
            disabled={acceptProposal.isPending || fileTextQ.isPending}
            onClick={() => acceptProposal.mutate()}
          >
            Accept
          </button>
          <button
            type="button"
            className="rounded-sm border border-border px-3 py-1.5 text-sm hover:bg-accent disabled:opacity-50"
            disabled={rejectProposal.isPending}
            onClick={() => rejectProposal.mutate()}
          >
            Reject
          </button>
          {acceptProposal.isError ? (
            <span className="self-center text-xs text-red-600">
              {(acceptProposal.error as Error)?.message ?? "Accept failed"}
            </span>
          ) : null}
        </div>
      ) : null}
    </div>
  );
}
