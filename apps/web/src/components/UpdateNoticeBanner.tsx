import { useQuery } from "@tanstack/react-query";
import { useEffect, useState, useCallback } from "react";
import { fetchPublicInstance } from "../api/instance";
import {
  fetchLatestUpdateInfo,
  isNewerThanCurrent,
} from "../api/updateCheck";
import {
  dismissUpdateNotice,
  getUpdateCheckOptIn,
  isUpdateDismissedForVersion,
  UPDATE_CHECK_OPT_IN_EVENT,
} from "../lib/updateCheckStorage";

export function UpdateNoticeBanner() {
  const [optIn, setOptIn] = useState(getUpdateCheckOptIn);
  const [justDismissed, setJustDismissed] = useState(false);

  useEffect(() => {
    const onChange = () => setOptIn(getUpdateCheckOptIn());
    window.addEventListener(UPDATE_CHECK_OPT_IN_EVENT, onChange);
    return () => window.removeEventListener(UPDATE_CHECK_OPT_IN_EVENT, onChange);
  }, []);

  const instanceQ = useQuery({
    queryKey: ["public-instance"],
    queryFn: fetchPublicInstance,
  });

  const inst = instanceQ.data;
  const canCheck =
    optIn &&
    !!inst &&
    (!!inst.update_manifest_url?.trim() ||
      !!inst.upstream_github_repo?.trim());

  const updateQ = useQuery({
    queryKey: [
      "update-check-remote",
      inst?.version,
      inst?.update_manifest_url,
      inst?.upstream_github_repo,
    ],
    queryFn: () => {
      if (!inst) {
        throw new Error("instance");
      }
      return fetchLatestUpdateInfo(inst);
    },
    enabled: canCheck && !!inst,
    staleTime: 24 * 60 * 60 * 1000,
    retry: 1,
  });

  const latest = updateQ.data;
  const current = inst?.version;

  useEffect(() => {
    setJustDismissed(false);
  }, [latest?.latestVersion]);

  const show =
    canCheck &&
    latest &&
    isNewerThanCurrent(current, latest.latestVersion) &&
    !isUpdateDismissedForVersion(latest.latestVersion) &&
    !justDismissed;

  const onDismiss = useCallback(() => {
    if (latest) {
      dismissUpdateNotice(latest.latestVersion);
    }
    setJustDismissed(true);
  }, [latest]);

  if (!show || !latest) {
    return null;
  }

  return (
    <div
      role="status"
      className="shrink-0 border-b border-border bg-muted/40 px-4 py-2 text-sm text-foreground"
    >
      <div className="mx-auto flex max-w-6xl flex-wrap items-center justify-between gap-2">
        <p className="text-muted-foreground">
          <span className="font-medium text-foreground">
            New version available
          </span>
          {`: ${latest.latestVersion}`}
          {current ? ` (you have ${current})` : null}
        </p>
        <div className="flex flex-wrap items-center gap-3">
          {latest.releaseNotesUrl ? (
            <a
              className="text-primary underline"
              href={latest.releaseNotesUrl}
              target="_blank"
              rel="noreferrer"
            >
              Release notes
            </a>
          ) : null}
          {latest.upgradeGuideUrl ? (
            <a
              className="text-primary underline"
              href={latest.upgradeGuideUrl}
              target="_blank"
              rel="noreferrer"
            >
              Upgrade guide
            </a>
          ) : null}
          <button
            type="button"
            className="rounded-sm border border-border bg-background px-2 py-1 text-xs hover:bg-accent"
            onClick={onDismiss}
          >
            Dismiss
          </button>
        </div>
      </div>
    </div>
  );
}
