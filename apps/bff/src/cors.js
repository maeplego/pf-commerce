/**
 * Browser Origin allowlist for the GraphQL BFF.
 * Requests without Origin (curl, Next server components) stay allowed.
 * `*` is explicit opt-in only.
 */
export function parseOrigins(raw) {
  return new Set(
    String(raw ?? "")
      .split(",")
      .map((s) => s.trim())
      .filter(Boolean),
  );
}

export function defaultBffCorsOrigin() {
  return process.env.COMMERCE_BFF_CORS_ORIGIN?.trim() || "http://localhost:3009";
}

export function graphqlOriginGate(reqOrigin, allowedRaw) {
  const allowed = parseOrigins(allowedRaw);
  const corsHeaders = {
    "Access-Control-Allow-Headers": "Content-Type",
    "Access-Control-Allow-Methods": "POST, OPTIONS",
  };
  if (allowed.has("*")) {
    return {
      ok: true,
      headers: { ...corsHeaders, "Access-Control-Allow-Origin": "*" },
    };
  }
  if (!reqOrigin) {
    return { ok: true, headers: {} };
  }
  if (allowed.has(reqOrigin)) {
    return {
      ok: true,
      headers: {
        ...corsHeaders,
        "Access-Control-Allow-Origin": reqOrigin,
        Vary: "Origin",
      },
    };
  }
  return { ok: false, headers: {} };
}
