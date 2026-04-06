import { describe, expect, it } from "vitest";
import { canonicalSignPayload, hmacSha256Hex } from "./crypto";

/** Must match apps/api/internal/provisioning/gateway_hmac_test.go */
const VECTOR_TS = 1700000000;
const VECTOR_METHOD = "POST";
const VECTOR_PATH = "/v1/claims";
const VECTOR_BODY = new TextEncoder().encode('{"slug":"acme","ipv4":"203.0.113.1"}');
const VECTOR_BODY_HASH = "349a1e120607a9074611fefdb92705eb3ab72cb158f304aa6035747e67d0be28";
const VECTOR_SECRET = "test-secret-key-for-vectors";
const VECTOR_SIG = "07714ec0d38ecb83efd662d8fc08e105000861beba6ef3fad043de968587d188";

describe("canonicalSignPayload", () => {
  it("matches Go test vector body hash line", async () => {
    const canonical = await canonicalSignPayload(
      VECTOR_TS,
      VECTOR_METHOD,
      VECTOR_PATH,
      VECTOR_BODY
    );
    expect(canonical).toBe(
      `${VECTOR_TS}\n${VECTOR_METHOD}\n${VECTOR_PATH}\n${VECTOR_BODY_HASH}`
    );
  });
});

describe("hmacSha256Hex", () => {
  it("matches Go test vector signature", async () => {
    const canonical = await canonicalSignPayload(
      VECTOR_TS,
      VECTOR_METHOD,
      VECTOR_PATH,
      VECTOR_BODY
    );
    const sig = await hmacSha256Hex(VECTOR_SECRET, canonical);
    expect(sig.toLowerCase()).toBe(VECTOR_SIG);
  });
});
