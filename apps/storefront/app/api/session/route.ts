import { NextResponse } from "next/server";

import { getCommerceSession } from "@/lib/session";

export async function GET() {
  const session = await getCommerceSession();
  return NextResponse.json({
    oidc: session.oidc,
    loggedIn: session.loggedIn,
    sub: session.sub,
    displayName: session.displayName,
    devMode: session.devMode,
    orgId: session.orgId,
    organizations: session.organizations,
  });
}
