/** Canonical signing string (must match Go provisioning.CanonicalSignPayload). */
export async function canonicalSignPayload(
  timestampUnix: number,
  method: string,
  path: string,
  body: ArrayBuffer | Uint8Array
): Promise<string> {
  const bodyBuf = body instanceof ArrayBuffer ? new Uint8Array(body) : body;
  const hashBuf = await crypto.subtle.digest("SHA-256", bodyBuf);
  const bodyHash = bufferToHex(hashBuf);
  return `${timestampUnix}\n${method.toUpperCase()}\n${path}\n${bodyHash}`;
}

function bufferToHex(buf: ArrayBuffer): string {
  return [...new Uint8Array(buf)].map((b) => b.toString(16).padStart(2, "0")).join("");
}

export async function hmacSha256Hex(keyUtf8: string, message: string): Promise<string> {
  const enc = new TextEncoder();
  const key = await crypto.subtle.importKey(
    "raw",
    enc.encode(keyUtf8),
    { name: "HMAC", hash: "SHA-256" },
    false,
    ["sign"]
  );
  const sig = await crypto.subtle.sign("HMAC", key, enc.encode(message));
  return bufferToHex(sig);
}

export async function verifyInstallHmac(
  installSecret: string,
  timestampUnix: number,
  method: string,
  path: string,
  body: ArrayBuffer,
  wantSigHex: string
): Promise<boolean> {
  const canonical = await canonicalSignPayload(timestampUnix, method, path, body);
  const got = (await hmacSha256Hex(installSecret, canonical)).toLowerCase();
  return timingSafeEqualHex(got, wantSigHex.trim().toLowerCase());
}

function timingSafeEqualHex(a: string, b: string): boolean {
  if (a.length !== b.length) return false;
  let diff = 0;
  for (let i = 0; i < a.length; i++) {
    diff |= a.charCodeAt(i) ^ b.charCodeAt(i);
  }
  return diff === 0;
}
