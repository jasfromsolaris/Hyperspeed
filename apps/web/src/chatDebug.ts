/** localStorage key: set to "1" to log chat / AI realtime in production builds. */
export const CHAT_DEBUG_STORAGE_KEY = "hyperspeed_debug_chat";

export function isChatDebugEnabled(): boolean {
  if (import.meta.env.DEV) return true;
  try {
    return globalThis.localStorage?.getItem(CHAT_DEBUG_STORAGE_KEY) === "1";
  } catch {
    return false;
  }
}

/** Tagged console logging for chat sends and org WebSocket events. */
export function chatDebug(...args: unknown[]): void {
  if (!isChatDebugEnabled()) return;
  console.info("[Hyperspeed chat]", ...args);
}

/**
 * Server-side mentions are only `<@uuid>` tokens inserted by the @ picker.
 * Plain `@DisplayName` does not enqueue AI staff — warn so it is not mistaken for a silent failure.
 */
export function warnIfNoUserMentionTokens(content: string): void {
  if (!content.includes("@")) return;
  if (/<@[0-9a-fA-F-]{36}>/.test(content)) return;
  console.warn(
    '[Hyperspeed chat] This message contains "@" but no user mention token (<@user-id>). AI staff is only triggered when you pick a user from the @ autocomplete, not when typing @Name by hand.',
  );
}
