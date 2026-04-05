import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { ArrowLeft, Database, Trash2, Upload } from "lucide-react";
import { useMemo, useState } from "react";
import { useNavigate, useParams } from "react-router-dom";
import { apiFetch } from "../api/http";

type DatasetFormat = "parquet" | "csv";

type SpaceDataset = {
  id: string;
  space_id: string;
  name: string;
  format: DatasetFormat;
  size_bytes?: number | null;
  schema_json?: { columns?: { name: string; type: string }[] } | null;
  row_count_estimate?: number | null;
  created_at: string;
};

type Preview = { columns: string[]; rows: string[][] };

export default function SpaceDatasetsPage() {
  const qc = useQueryClient();
  const navigate = useNavigate();
  const { orgId, projectId } = useParams<{ orgId: string; projectId: string }>();
  const [selectedId, setSelectedId] = useState<string | null>(null);
  const [queryBody, setQueryBody] = useState(
    () => `{\n  "columns": [],\n  "filters": [],\n  "limit": 100,\n  "offset": 0\n}`,
  );
  const [queryResult, setQueryResult] = useState<Preview | null>(null);

  const featuresQ = useQuery({
    queryKey: ["org-features", orgId],
    enabled: !!orgId,
    queryFn: async () => {
      const res = await apiFetch(`/api/v1/organizations/${orgId}/features`);
      if (!res.ok) throw new Error("features");
      const j = (await res.json()) as { features: { datasets_enabled: boolean } };
      return j.features;
    },
  });
  const datasetsEnabled = !!featuresQ.data?.datasets_enabled;

  const projectQ = useQuery({
    queryKey: ["project", orgId, projectId],
    enabled: !!orgId && !!projectId,
    queryFn: async () => {
      const res = await apiFetch(`/api/v1/organizations/${orgId}/spaces/${projectId}`);
      if (!res.ok) throw new Error("project");
      return res.json() as Promise<{ name: string }>;
    },
  });

  const listQ = useQuery({
    queryKey: ["datasets", projectId],
    enabled: !!orgId && !!projectId && datasetsEnabled,
    queryFn: async () => {
      const res = await apiFetch(
        `/api/v1/organizations/${orgId}/spaces/${projectId}/datasets`,
      );
      if (!res.ok) throw new Error("datasets");
      const j = (await res.json()) as { datasets: SpaceDataset[] };
      return j.datasets;
    },
  });

  const previewQ = useQuery({
    queryKey: ["dataset-preview", projectId, selectedId],
    enabled: !!orgId && !!projectId && !!selectedId && datasetsEnabled,
    queryFn: async () => {
      const res = await apiFetch(
        `/api/v1/organizations/${orgId}/spaces/${projectId}/datasets/${selectedId}/preview?limit=50`,
      );
      if (!res.ok) throw new Error("preview");
      const j = (await res.json()) as { preview: Preview };
      return j.preview;
    },
  });

  const selected = useMemo(
    () => listQ.data?.find((d) => d.id === selectedId) ?? null,
    [listQ.data, selectedId],
  );

  const upload = useMutation({
    mutationFn: async (vars: { file: File; format: DatasetFormat; name: string }) => {
      const init = await apiFetch(
        `/api/v1/organizations/${orgId}/spaces/${projectId}/datasets/upload/init`,
        {
          method: "POST",
          json: { name: vars.name, format: vars.format },
        },
      );
      if (!init.ok) throw new Error("init");
      const j = (await init.json()) as { dataset: SpaceDataset; upload_url: string };
      const put = await fetch(j.upload_url, {
        method: "PUT",
        body: vars.file,
        headers: {
          "Content-Type": vars.format === "csv" ? "text/csv" : "application/octet-stream",
        },
      });
      if (!put.ok) throw new Error("upload");
      const done = await apiFetch(
        `/api/v1/organizations/${orgId}/spaces/${projectId}/datasets/upload/complete`,
        { method: "POST", json: { dataset_id: j.dataset.id } },
      );
      if (!done.ok) throw new Error("complete");
      return done.json() as Promise<{ dataset: SpaceDataset }>;
    },
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ["datasets", projectId] });
    },
  });

  const runQuery = useMutation({
    mutationFn: async () => {
      if (!selectedId) throw new Error("no dataset");
      let body: unknown;
      try {
        body = JSON.parse(queryBody) as unknown;
      } catch {
        throw new Error("invalid JSON");
      }
      const res = await apiFetch(
        `/api/v1/organizations/${orgId}/spaces/${projectId}/datasets/${selectedId}/query`,
        { method: "POST", json: body },
      );
      if (!res.ok) throw new Error("query");
      const j = (await res.json()) as { result: Preview };
      return j.result;
    },
    onSuccess: (r) => setQueryResult(r),
  });

  const del = useMutation({
    mutationFn: async (id: string) => {
      const res = await apiFetch(
        `/api/v1/organizations/${orgId}/spaces/${projectId}/datasets/${id}`,
        { method: "DELETE" },
      );
      if (!res.ok) throw new Error("delete");
    },
    onSuccess: () => {
      setSelectedId(null);
      void qc.invalidateQueries({ queryKey: ["datasets", projectId] });
    },
  });

  return (
    <div className="min-h-0 flex-1 overflow-y-auto bg-background">
      <div className="flex h-full min-h-0 flex-col">
        <header className="border-b border-border px-4 py-3">
          <div className="flex flex-wrap items-center justify-between gap-3">
            <div className="min-w-0">
              <p className="text-xs font-medium uppercase tracking-[0.15em] text-muted-foreground">
                {projectQ.data?.name ?? "Space"}
              </p>
              <h1 className="mt-1 flex items-center gap-2 truncate text-base font-semibold text-foreground">
                <Database className="h-5 w-5 shrink-0 opacity-80" />
                Datasets
              </h1>
            </div>
            <div className="flex flex-wrap items-center gap-2">
              <button
                type="button"
                className="inline-flex items-center gap-2 rounded-sm border border-border bg-card px-3 py-2 text-sm text-foreground hover:bg-accent"
                onClick={() => void navigate(`/o/${orgId}/p/${projectId}/files`)}
              >
                <ArrowLeft className="h-4 w-4 opacity-80" />
                Files
              </button>
              <label className="inline-flex cursor-pointer items-center gap-2 rounded-sm bg-primary px-3 py-2 text-sm font-medium text-primary-foreground hover:bg-primary/90">
                <Upload className="h-4 w-4" />
                Upload
                <input
                  type="file"
                  accept=".csv,.parquet,text/csv,application/octet-stream"
                  className="hidden"
                  disabled={upload.isPending}
                  onChange={(e) => {
                    const f = e.target.files?.[0];
                    e.target.value = "";
                    if (!f || !orgId || !projectId) return;
                    const lower = f.name.toLowerCase();
                    const format: DatasetFormat = lower.endsWith(".csv") ? "csv" : "parquet";
                    const name = f.name.replace(/\.(csv|parquet)$/i, "") || f.name;
                    upload.mutate({ file: f, format, name });
                  }}
                />
              </label>
            </div>
          </div>
          <p className="mt-2 text-xs text-muted-foreground">
            Parquet or CSV, server-side preview and filtered queries (row limits enforced).
          </p>
        </header>

        {!featuresQ.isPending && !datasetsEnabled ? (
          <div className="p-4">
            <div className="rounded-sm border border-border bg-card p-4">
              <p className="text-sm text-foreground">Datasets are disabled for this workspace.</p>
              <p className="mt-1 text-xs text-muted-foreground">
                A workspace admin can enable this in Settings.
              </p>
            </div>
          </div>
        ) : null}

        {datasetsEnabled ? (
          <div className="grid min-h-0 flex-1 gap-4 p-4 lg:grid-cols-[minmax(200px,280px)_1fr]">
          <aside className="min-h-0 overflow-y-auto rounded-sm border border-border bg-card p-2">
            <div className="text-xs font-semibold uppercase tracking-wider text-muted-foreground">
              In this space
            </div>
            <ul className="mt-2 space-y-1">
              {(listQ.data ?? []).map((d) => (
                <li key={d.id}>
                  <button
                    type="button"
                    className={[
                      "flex w-full items-center justify-between gap-2 rounded-sm px-2 py-1.5 text-left text-sm",
                      selectedId === d.id ? "bg-accent/50 text-foreground" : "hover:bg-accent/30",
                    ].join(" ")}
                    onClick={() => setSelectedId(d.id)}
                  >
                    <span className="min-w-0 truncate">{d.name}</span>
                    <span className="shrink-0 text-[10px] uppercase text-muted-foreground">
                      {d.format}
                    </span>
                  </button>
                </li>
              ))}
            </ul>
            {listQ.isPending ? (
              <p className="mt-2 text-xs text-muted-foreground">Loading…</p>
            ) : null}
            {!listQ.isPending && (listQ.data?.length ?? 0) === 0 ? (
              <p className="mt-2 text-xs text-muted-foreground">No datasets yet.</p>
            ) : null}
          </aside>

          <main className="min-h-0 overflow-y-auto rounded-sm border border-border bg-card p-4">
            {!selected ? (
              <p className="text-sm text-muted-foreground">Select a dataset to preview and query.</p>
            ) : (
              <div className="space-y-4">
                <div className="flex flex-wrap items-start justify-between gap-2">
                  <div>
                    <h2 className="text-lg font-semibold text-foreground">{selected.name}</h2>
                    <p className="text-xs text-muted-foreground">
                      {selected.size_bytes != null
                        ? `${(selected.size_bytes / 1024).toFixed(1)} KB`
                        : "—"}{" "}
                      · rows ~{selected.row_count_estimate ?? "?"}
                    </p>
                  </div>
                  <button
                    type="button"
                    className="inline-flex items-center gap-1 rounded-sm border border-destructive/50 px-2 py-1 text-xs text-destructive hover:bg-destructive/10"
                    disabled={del.isPending}
                    onClick={() => {
                      if (confirm(`Delete dataset “${selected.name}”?`)) del.mutate(selected.id);
                    }}
                  >
                    <Trash2 className="h-3.5 w-3.5" />
                    Delete
                  </button>
                </div>

                {selected.schema_json?.columns?.length ? (
                  <div>
                    <div className="text-xs font-semibold text-muted-foreground">Columns</div>
                    <p className="mt-1 text-sm text-foreground">
                      {selected.schema_json.columns.map((c) => `${c.name} (${c.type})`).join(", ")}
                    </p>
                  </div>
                ) : null}

                <div>
                  <div className="text-xs font-semibold text-muted-foreground">Preview</div>
                  {previewQ.isPending ? (
                    <p className="mt-2 text-sm text-muted-foreground">Loading preview…</p>
                  ) : previewQ.data ? (
                    <div className="mt-2 overflow-x-auto">
                      <table className="w-full border-collapse text-sm">
                        <thead>
                          <tr>
                            {previewQ.data.columns.map((c) => (
                              <th
                                key={c}
                                className="border border-border bg-muted/40 px-2 py-1 text-left font-medium"
                              >
                                {c}
                              </th>
                            ))}
                          </tr>
                        </thead>
                        <tbody>
                          {previewQ.data.rows.map((row, ri) => (
                            <tr key={ri}>
                              {row.map((cell, ci) => (
                                <td key={ci} className="border border-border px-2 py-1">
                                  {cell}
                                </td>
                              ))}
                            </tr>
                          ))}
                        </tbody>
                      </table>
                    </div>
                  ) : (
                    <p className="mt-2 text-sm text-muted-foreground">No preview.</p>
                  )}
                </div>

                <div>
                  <div className="flex items-center justify-between gap-2">
                    <div className="text-xs font-semibold text-muted-foreground">Query (JSON DSL)</div>
                    <button
                      type="button"
                      className="rounded-sm bg-primary px-3 py-1 text-xs font-medium text-primary-foreground hover:bg-primary/90"
                      disabled={runQuery.isPending}
                      onClick={() => runQuery.mutate()}
                    >
                      Run
                    </button>
                  </div>
                  <textarea
                    className="mt-2 h-36 w-full rounded-sm border border-input bg-background p-2 font-mono text-xs text-foreground"
                    value={queryBody}
                    onChange={(e) => setQueryBody(e.target.value)}
                    spellCheck={false}
                  />
                  {runQuery.isError ? (
                    <p className="mt-1 text-xs text-destructive">
                      {(runQuery.error as Error)?.message ?? "Query failed"}
                    </p>
                  ) : null}
                  {queryResult ? (
                    <div className="mt-3 overflow-x-auto">
                      <table className="w-full border-collapse text-sm">
                        <thead>
                          <tr>
                            {queryResult.columns.map((c) => (
                              <th
                                key={c}
                                className="border border-border bg-muted/40 px-2 py-1 text-left font-medium"
                              >
                                {c}
                              </th>
                            ))}
                          </tr>
                        </thead>
                        <tbody>
                          {queryResult.rows.map((row, ri) => (
                            <tr key={ri}>
                              {row.map((cell, ci) => (
                                <td key={ci} className="border border-border px-2 py-1">
                                  {cell}
                                </td>
                              ))}
                            </tr>
                          ))}
                        </tbody>
                      </table>
                    </div>
                  ) : null}
                </div>
              </div>
            )}
          </main>
          </div>
        ) : null}
      </div>
    </div>
  );
}
