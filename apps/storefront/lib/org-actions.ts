"use server";

import { revalidatePath } from "next/cache";

import { getCommerceSession } from "./session";

export async function switchActiveOrg(orgId: string, devUser?: string) {
  const session = await getCommerceSession(devUser);
  const next = orgId.trim();
  if (!next) return;

  if (session.devMode) {
    const { cookies } = await import("next/headers");
    const jar = await cookies();
    jar.set("dev_org", next, { httpOnly: true, sameSite: "lax", path: "/", maxAge: 60 * 60 * 24 * 7 });
    revalidatePath("/");
    return;
  }

  if (!session.accessToken) throw new Error("unauthorized");
  const { internalBase, clientId } = await import("./oidc/env");
  const { readCookie } = await import("./oidc/cookies");
  const switchRes = await fetch(`${internalBase()}/v1/active-org`, {
    method: "PUT",
    headers: {
      Authorization: `Bearer ${session.accessToken}`,
      "Content-Type": "application/json",
    },
    body: JSON.stringify({ orgId: next }),
    cache: "no-store",
  });
  if (!switchRes.ok) {
    throw new Error(await switchRes.text());
  }

  const refresh = await readCookie("rp_refresh");
  if (refresh) {
    const body = new URLSearchParams({
      grant_type: "refresh_token",
      client_id: clientId(),
      refresh_token: refresh,
    });
    const tokenRes = await fetch(`${internalBase()}/token`, {
      method: "POST",
      headers: { "Content-Type": "application/x-www-form-urlencoded" },
      body,
      cache: "no-store",
    });
    if (tokenRes.ok) {
      const tokens = (await tokenRes.json()) as {
        access_token?: string;
        id_token?: string;
        refresh_token?: string;
      };
      const { cookies } = await import("next/headers");
      const { cookieKey } = await import("./oidc/cookies");
      const jar = await cookies();
      const base = { httpOnly: true, sameSite: "lax" as const, path: "/", maxAge: 60 * 60 * 24 * 7 };
      if (tokens.access_token) jar.set(cookieKey("rp_access"), tokens.access_token, base);
      if (tokens.id_token) jar.set(cookieKey("rp_id"), tokens.id_token, base);
      if (tokens.refresh_token) jar.set(cookieKey("rp_refresh"), tokens.refresh_token, base);
    }
  }
  revalidatePath("/");
}
