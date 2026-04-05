import { useQueries, useQuery } from "@tanstack/react-query";
import { ChevronLeft, ChevronRight } from "lucide-react";
import { useMemo, useState } from "react";
import { Link } from "react-router-dom";
import { apiFetch } from "../api/http";
import { fetchMyAssignedTasks } from "../api/tasks";
import { fetchOrganizationsList } from "../api/orgs";
import type { MyAssignedTask, Project } from "../api/types";
import { orgDotStyle, orgHue } from "../lib/orgColor";

const WEEKDAYS = ["Sun", "Mon", "Tue", "Wed", "Thu", "Fri", "Sat"];

function localDateKey(iso: string): string {
  const d = new Date(iso);
  const y = d.getFullYear();
  const m = String(d.getMonth() + 1).padStart(2, "0");
  const day = String(d.getDate()).padStart(2, "0");
  return `${y}-${m}-${day}`;
}

function keyFromParts(y: number, monthIndex: number, day: number): string {
  return `${y}-${String(monthIndex + 1).padStart(2, "0")}-${String(day).padStart(2, "0")}`;
}

/** Month grid cells: each cell is a calendar day (may be adjacent month). */
function monthGridCells(year: number, monthIndex: number) {
  const first = new Date(year, monthIndex, 1);
  const daysInMonth = new Date(year, monthIndex + 1, 0).getDate();
  const startWeekday = first.getDay();
  type Cell = { y: number; m: number; d: number; inMonth: boolean };
  const cells: Cell[] = [];

  const prevLast = new Date(year, monthIndex, 0);
  const prevYear = prevLast.getFullYear();
  const prevMonth = prevLast.getMonth();
  const prevDays = prevLast.getDate();
  for (let i = 0; i < startWeekday; i++) {
    const d = prevDays - startWeekday + i + 1;
    cells.push({ y: prevYear, m: prevMonth, d, inMonth: false });
  }
  for (let d = 1; d <= daysInMonth; d++) {
    cells.push({ y: year, m: monthIndex, d, inMonth: true });
  }
  let nextY = year;
  let nextM = monthIndex + 1;
  if (nextM > 11) {
    nextM = 0;
    nextY++;
  }
  let nextD = 1;
  while (cells.length % 7 !== 0) {
    cells.push({ y: nextY, m: nextM, d: nextD++, inMonth: false });
  }
  return cells;
}

export default function DashboardPage() {
  const orgsQ = useQuery({
    queryKey: ["orgs"],
    queryFn: async () => {
      try {
        return await fetchOrganizationsList();
      } catch {
        throw new Error("Failed to load workspaces");
      }
    },
  });

  const orgs = orgsQ.data?.organizations ?? [];

  const projectQueries = useQueries({
    queries: orgs.map((o) => ({
      queryKey: ["projects", o.id] as const,
      queryFn: async () => {
        const res = await apiFetch(`/api/v1/organizations/${o.id}/spaces`);
        if (res.status === 403) {
          return [];
        }
        if (!res.ok) {
          throw new Error("projects");
        }
        const j = (await res.json()) as { spaces: Project[] };
        return j.spaces.map((p) => ({ ...p, orgId: o.id }));
      },
      enabled: orgs.length > 0,
    })),
  });

  const allProjectsFlat = useMemo(() => {
    const out: Array<Project & { orgId: string }> = [];
    for (const q of projectQueries) {
      for (const p of q.data ?? []) {
        out.push(p);
      }
    }
    return out;
  }, [projectQueries]);

  const projectCount = allProjectsFlat.length;

  const myTaskQueries = useQueries({
    queries: orgs.map((o) => ({
      queryKey: ["my-tasks", o.id] as const,
      queryFn: () => fetchMyAssignedTasks(o.id),
      enabled: orgs.length > 0,
    })),
  });

  const myTasksAll = useMemo(() => {
    const rows: MyAssignedTask[] = [];
    for (const q of myTaskQueries) {
      for (const t of q.data ?? []) {
        rows.push(t);
      }
    }
    rows.sort(
      (a, b) =>
        new Date(b.updated_at).getTime() - new Date(a.updated_at).getTime(),
    );
    return rows;
  }, [myTaskQueries]);

  const myTasks = useMemo(() => myTasksAll.slice(0, 24), [myTasksAll]);

  const tasksByDueDate = useMemo(() => {
    const m = new Map<string, MyAssignedTask[]>();
    for (const t of myTasksAll) {
      if (!t.due_at) continue;
      const key = localDateKey(t.due_at);
      const list = m.get(key) ?? [];
      list.push(t);
      m.set(key, list);
    }
    return m;
  }, [myTasksAll]);

  const dueCount = useMemo(
    () => myTasksAll.filter((t) => t.due_at).length,
    [myTasksAll],
  );

  const now = new Date();
  const [viewYear, setViewYear] = useState(now.getFullYear());
  const [viewMonth, setViewMonth] = useState(now.getMonth());

  const calendarCells = useMemo(
    () => monthGridCells(viewYear, viewMonth),
    [viewYear, viewMonth],
  );

  function prevMonth() {
    setViewMonth((m) => {
      if (m === 0) {
        setViewYear((y) => y - 1);
        return 11;
      }
      return m - 1;
    });
  }

  function nextMonth() {
    setViewMonth((m) => {
      if (m === 11) {
        setViewYear((y) => y + 1);
        return 0;
      }
      return m + 1;
    });
  }

  const myTasksLoading = orgs.length > 0 && myTaskQueries.some((q) => q.isLoading);

  const monthLabel = new Date(viewYear, viewMonth, 1).toLocaleString(undefined, {
    month: "long",
    year: "numeric",
  });

  return (
    <div className="min-h-0 flex-1 overflow-y-auto bg-background px-4 py-8 md:px-8">
      <div className="mx-auto max-w-6xl">
        <header className="mb-8">
          <p className="text-xs font-medium uppercase tracking-[0.2em] text-muted-foreground">
            Dashboard
          </p>
          <h1 className="mt-1 text-2xl font-semibold tracking-tight text-foreground">
            Hyperspeed
          </h1>
          <p className="mt-1 text-sm text-muted-foreground">
            Workspaces, spaces, and tasks at a glance.
          </p>
        </header>

        <div className="grid grid-cols-1 gap-4 md:grid-cols-2 xl:grid-cols-4">
          <MetricCard
            label="Workspaces"
            value={orgs.length}
            loading={orgsQ.isLoading}
          />
          <MetricCard
            label="Spaces"
            value={projectCount}
            loading={projectQueries.some((q) => q.isLoading)}
          />
          <MetricCard
            label="Assigned to me"
            value={myTasksAll.length}
            hint="Across orgs you can access"
            loading={myTasksLoading}
          />
          <MetricCard
            label="With due date"
            value={dueCount}
            hint="Assigned tasks that have a due date"
            loading={myTasksLoading}
          />
        </div>

        <div className="mt-10 grid grid-cols-1 gap-6 lg:grid-cols-2">
          <section className="rounded-sm border border-border bg-card p-4">
            <h2 className="text-xs font-semibold uppercase tracking-[0.15em] text-muted-foreground">
              My tasks
            </h2>
            {myTasks.length === 0 && !myTasksLoading ? (
              <p className="mt-4 text-sm text-muted-foreground">
                No tasks assigned to you yet.
              </p>
            ) : (
              <ul className="mt-3 space-y-2">
                {myTasks.map((t) => (
                  <li key={`${t.organization_id}-${t.id}`}>
                    <Link
                      to={`/o/${t.organization_id}/p/${t.space_id}/b/${t.board_id}?task=${encodeURIComponent(t.id)}`}
                      className="block rounded-sm border border-transparent px-2 py-2 transition-colors hover:border-border hover:bg-accent/30"
                    >
                      <div className="flex items-start gap-2">
                        <span
                          className="mt-1.5 h-1.5 w-1.5 shrink-0 rounded-full"
                          style={orgDotStyle(t.organization_id)}
                          aria-hidden
                        />
                        <div className="min-w-0 flex-1">
                          <div className="font-medium text-card-foreground">
                            {t.title}
                          </div>
                          <div className="text-xs text-muted-foreground">
                            {t.space_name} ·{" "}
                            {new Date(t.updated_at).toLocaleString()}
                          </div>
                        </div>
                      </div>
                    </Link>
                  </li>
                ))}
              </ul>
            )}
          </section>
          <section className="overflow-hidden rounded-lg border border-border/90 bg-card">
            <div className="flex flex-wrap items-center justify-between gap-x-3 gap-y-2 border-b border-border/70 px-3 py-2.5">
              <div className="min-w-0 flex-1">
                <h2 className="text-[10px] font-semibold uppercase tracking-[0.18em] text-muted-foreground">
                  Due dates
                </h2>
                <p className="mt-0.5 truncate text-[10px] text-muted-foreground/90">
                  Assigned tasks · accent = workspace
                </p>
              </div>
              <div className="flex items-center gap-1">
                <button
                  type="button"
                  className="inline-flex h-7 w-7 shrink-0 items-center justify-center rounded-md text-muted-foreground transition hover:bg-muted hover:text-foreground"
                  aria-label="Previous month"
                  onClick={prevMonth}
                >
                  <ChevronLeft className="h-3.5 w-3.5" />
                </button>
                <span className="min-w-[9.5rem] text-center text-xs font-semibold tabular-nums text-foreground">
                  {monthLabel}
                </span>
                <button
                  type="button"
                  className="inline-flex h-7 w-7 shrink-0 items-center justify-center rounded-md text-muted-foreground transition hover:bg-muted hover:text-foreground"
                  aria-label="Next month"
                  onClick={nextMonth}
                >
                  <ChevronRight className="h-3.5 w-3.5" />
                </button>
              </div>
            </div>

            {myTasksLoading ? (
              <p className="py-6 text-center text-xs text-muted-foreground">Loading…</p>
            ) : (
              <div className="p-2">
                <div className="mb-1 grid grid-cols-7 gap-px">
                  {WEEKDAYS.map((d) => (
                    <div
                      key={d}
                      className="py-1 text-center text-[9px] font-medium uppercase tracking-wide text-muted-foreground"
                    >
                      {d.slice(0, 2)}
                    </div>
                  ))}
                </div>
                <div className="grid grid-cols-7 gap-px rounded-md bg-border/60 p-px">
                  {calendarCells.map((cell, idx) => {
                    const key = keyFromParts(cell.y, cell.m, cell.d);
                    const dayTasks = tasksByDueDate.get(key) ?? [];
                    const hasTasks = dayTasks.length > 0;
                    const isToday =
                      cell.inMonth &&
                      cell.y === now.getFullYear() &&
                      cell.m === now.getMonth() &&
                      cell.d === now.getDate();
                    return (
                      <div
                        key={`${key}-${idx}`}
                        className={[
                          "flex min-h-[4.25rem] flex-col bg-card p-1 sm:min-h-[4.5rem]",
                          !cell.inMonth && "bg-muted/25 text-muted-foreground",
                          cell.inMonth && !hasTasks && "hover:bg-muted/30",
                          cell.inMonth && hasTasks && "bg-primary/[0.06]",
                          isToday && "ring-1 ring-inset ring-primary/70",
                        ]
                          .filter(Boolean)
                          .join(" ")}
                      >
                        <div
                          className={[
                            "mb-0.5 flex h-5 w-5 shrink-0 items-center justify-center rounded text-[10px] tabular-nums leading-none",
                            !cell.inMonth && "text-muted-foreground",
                            cell.inMonth && !isToday && "font-medium text-foreground",
                            isToday &&
                              "bg-primary font-semibold text-primary-foreground",
                          ].join(" ")}
                        >
                          {cell.d}
                        </div>
                        <ul className="flex min-h-0 flex-1 flex-col gap-0.5 overflow-hidden">
                          {dayTasks.slice(0, 2).map((t) => (
                            <li key={t.id} className="min-w-0">
                              <Link
                                to={`/o/${t.organization_id}/p/${t.space_id}/b/${t.board_id}?task=${encodeURIComponent(t.id)}`}
                                className="block w-full min-w-0 truncate rounded border border-transparent bg-background/90 py-0.5 pl-1 pr-0.5 text-left text-[10px] font-semibold leading-tight text-foreground transition hover:border-border hover:bg-muted/80 dark:bg-background/50"
                                style={{
                                  borderLeftWidth: 2,
                                  borderLeftStyle: "solid",
                                  borderLeftColor: `hsl(${orgHue(t.organization_id)} 55% 48%)`,
                                }}
                                title={t.title}
                                onClick={(e) => e.stopPropagation()}
                              >
                                {t.title}
                              </Link>
                            </li>
                          ))}
                          {dayTasks.length > 2 ? (
                            <li>
                              <span className="text-[9px] font-medium tabular-nums text-muted-foreground">
                                +{dayTasks.length - 2}
                              </span>
                            </li>
                          ) : null}
                        </ul>
                      </div>
                    );
                  })}
                </div>
                {dueCount === 0 ? (
                  <p className="mt-2 rounded border border-dashed border-border/80 bg-muted/15 px-2 py-2 text-center text-[10px] leading-snug text-muted-foreground">
                    No due dates yet — add one on an assigned task.
                  </p>
                ) : null}
              </div>
            )}
          </section>
        </div>

        <section className="mt-10">
          <h2 className="text-xs font-semibold uppercase tracking-[0.15em] text-muted-foreground">
            Workspaces
          </h2>
          <ul className="mt-3 space-y-2">
            {orgs.map((o) => (
              <li key={o.id}>
                <Link
                  to={`/o/${o.id}`}
                  className="flex items-center gap-2 rounded-sm border border-border bg-card px-4 py-3 transition-colors hover:border-muted-foreground/40"
                >
                  <span
                    className="h-2 w-2 shrink-0 rounded-full"
                    style={orgDotStyle(o.id)}
                    aria-hidden
                  />
                  <span className="font-medium text-card-foreground">{o.name}</span>
                  <span className="text-xs text-muted-foreground">{o.slug}</span>
                </Link>
              </li>
            ))}
          </ul>
          {orgs.length === 0 && !orgsQ.isLoading && (
            <p className="mt-4 text-sm text-muted-foreground">
              No workspaces visible. If this is a new server, register the first
              account from the sign-up page to create the workspace. Otherwise ask
              an administrator for an invite.
            </p>
          )}
        </section>
      </div>
    </div>
  );
}

function MetricCard(props: {
  label: string;
  value: number;
  hint?: string;
  loading?: boolean;
}) {
  return (
    <div className="rounded-sm border border-border bg-card px-4 py-3">
      <p className="text-[10px] font-semibold uppercase tracking-[0.15em] text-muted-foreground">
        {props.label}
      </p>
      <p className="mt-2 text-2xl font-semibold tabular-nums text-card-foreground">
        {props.loading ? "—" : props.value}
      </p>
      {props.hint && (
        <p className="mt-1 text-xs text-muted-foreground">{props.hint}</p>
      )}
    </div>
  );
}
