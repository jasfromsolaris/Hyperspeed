import { useEffect, useRef, useState } from "react";
import { apiFetch } from "../api/http";

export type LocalPresence = "online" | "away";

/**
 * Client-side idle detection + server-side last_seen ping.
 * - away after 2 minutes of inactivity while tab visible
 * - pings server every 15s while visible + on activity (debounced)
 */
export function usePresencePing(enabled: boolean) {
  const [localPresence, setLocalPresence] = useState<LocalPresence>("online");
  const lastActivityRef = useRef<number>(Date.now());
  const pingInFlightRef = useRef(false);
  const debounceRef = useRef<number | null>(null);

  useEffect(() => {
    if (!enabled) {
      return;
    }

    function markActivity() {
      lastActivityRef.current = Date.now();
      if (localPresence !== "online") {
        setLocalPresence("online");
      }
      if (debounceRef.current) {
        window.clearTimeout(debounceRef.current);
      }
      debounceRef.current = window.setTimeout(() => {
        void ping();
      }, 800);
    }

    function onVisibilityChange() {
      if (document.visibilityState === "visible") {
        markActivity();
      }
    }

    async function ping() {
      if (pingInFlightRef.current) {
        return;
      }
      pingInFlightRef.current = true;
      try {
        await apiFetch("/api/v1/presence/ping", { method: "POST" });
      } finally {
        pingInFlightRef.current = false;
      }
    }

    const activityEvents: (keyof WindowEventMap)[] = [
      "mousemove",
      "keydown",
      "pointerdown",
      "scroll",
    ];
    for (const ev of activityEvents) {
      window.addEventListener(ev, markActivity, { passive: true });
    }
    document.addEventListener("visibilitychange", onVisibilityChange);

    // Kick off immediately.
    void ping();

    const pingInterval = window.setInterval(() => {
      if (document.visibilityState !== "visible") {
        return;
      }
      void ping();
    }, 15000);

    const idleInterval = window.setInterval(() => {
      if (document.visibilityState !== "visible") {
        return;
      }
      const idleForMs = Date.now() - lastActivityRef.current;
      if (idleForMs >= 120000 && localPresence !== "away") {
        setLocalPresence("away");
      }
    }, 1000);

    return () => {
      for (const ev of activityEvents) {
        window.removeEventListener(ev, markActivity);
      }
      document.removeEventListener("visibilitychange", onVisibilityChange);
      window.clearInterval(pingInterval);
      window.clearInterval(idleInterval);
      if (debounceRef.current) {
        window.clearTimeout(debounceRef.current);
      }
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [enabled, localPresence]);

  return { localPresence };
}

