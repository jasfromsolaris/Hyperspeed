const OPT_IN_KEY = "hyperspeed:updateCheckOptIn";
const DISMISS_PREFIX = "hyperspeed:updateDismissed:";

export const UPDATE_CHECK_OPT_IN_EVENT = "hs:updateCheckOptIn";

export function getUpdateCheckOptIn(): boolean {
  try {
    return localStorage.getItem(OPT_IN_KEY) === "1";
  } catch {
    return false;
  }
}

export function setUpdateCheckOptIn(value: boolean): void {
  try {
    localStorage.setItem(OPT_IN_KEY, value ? "1" : "0");
    window.dispatchEvent(new CustomEvent(UPDATE_CHECK_OPT_IN_EVENT));
  } catch {
    /* ignore */
  }
}

export function isUpdateDismissedForVersion(latestVersion: string): boolean {
  try {
    return localStorage.getItem(DISMISS_PREFIX + latestVersion) === "1";
  } catch {
    return false;
  }
}

export function dismissUpdateNotice(latestVersion: string): void {
  try {
    localStorage.setItem(DISMISS_PREFIX + latestVersion, "1");
  } catch {
    /* ignore */
  }
}
