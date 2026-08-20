import { NextResponse } from "next/server";

import { readCookie } from "@/lib/oidc/cookies";
import { internalBase, oidcEnabled } from "@/lib/oidc/env";

export async function GET() {
  if (!oidcEnabled()) {
    return NextResponse.json({ oidc: false, loggedIn: true, sub: null, displayName: null, devMode: true });
  }
  const access = await readCookie("rp_access");
  if (!access) {
    return NextResponse.json({ oidc: true, loggedIn: false, sub: null, displayName: null, devMode: false });
  }
  const res = await fetch(`${internalBase()}/userinfo`, {
    headers: { Authorization: `Bearer ${access}` },
    cache: "no-store",
  });
  if (!res.ok) {
    return NextResponse.json({ oidc: true, loggedIn: false, sub: null, displayName: null, devMode: false });
  }
  const ui = (await res.json()) as { sub?: string; name?: string; email?: string };
  if (!ui.sub) {
    return NextResponse.json({ oidc: true, loggedIn: false, sub: null, displayName: null, devMode: false });
  }
  return NextResponse.json({
    oidc: true,
    loggedIn: true,
    sub: ui.sub,
    displayName: ui.name || ui.email || ui.sub,
    devMode: false,
  });
}
