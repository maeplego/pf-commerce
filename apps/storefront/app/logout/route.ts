import { NextRequest, NextResponse } from "next/server";

import { readCookie, setOn } from "../../lib/oidc/cookies";
import { clientId, issuer, oidcEnabled, postLogoutRedirectUri, publicOrigin } from "../../lib/oidc/env";
import { randomString } from "../../lib/oidc/pkce";

export async function GET(req: NextRequest) {
  const origin = publicOrigin(req);
  if (!oidcEnabled()) {
    return NextResponse.redirect(new URL("/", origin));
  }
  const idToken = await readCookie("rp_id");
  const res = NextResponse.redirect(
    idToken
      ? `${issuer()}/end-session?${new URLSearchParams({
          client_id: clientId(),
          post_logout_redirect_uri: postLogoutRedirectUri(),
          id_token_hint: idToken,
          state: randomString(8),
        }).toString()}`
      : new URL("/logged-out", origin).toString(),
    { status: 303 },
  );
  setOn(res, "rp_access", "", 0);
  setOn(res, "rp_id", "", 0);
  setOn(res, "rp_refresh", "", 0);
  return res;
}
