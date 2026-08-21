"use client";

import { useEffect, useState } from "react";

import { OrgSwitcher } from "@/components/OrgSwitcher";
import { switchActiveOrg } from "@/lib/org-actions";

type OrgOption = { orgId: string; orgName: string; role: string };

type Session = {
  oidc: boolean;
  loggedIn: boolean;
  sub: string | null;
  displayName: string | null;
  devMode: boolean;
  orgId?: string;
  organizations?: OrgOption[];
};

export function AuthBar() {
  const [session, setSession] = useState<Session | null>(null);

  const reload = () => {
    fetch("/api/session", { credentials: "same-origin", cache: "no-store" })
      .then((res) => res.json())
      .then((body) => setSession(body as Session))
      .catch(() => setSession(null));
  };

  useEffect(() => {
    reload();
  }, []);

  const onSwitch = async (orgId: string) => {
    await switchActiveOrg(orgId);
    reload();
    window.location.reload();
  };

  if (!session) {
    return <span className="auth-bar muted">…</span>;
  }

  const switcher =
    session.organizations && session.organizations.length > 0 ? (
      <OrgSwitcher currentOrgId={session.orgId} organizations={session.organizations} onSwitch={onSwitch} />
    ) : null;

  if (!session.oidc) {
    return (
      <span className="auth-bar" style={{ display: "inline-flex", gap: "0.75rem", alignItems: "center" }}>
        {switcher}
        <span className="muted">dev-auth</span>
      </span>
    );
  }
  if (!session.loggedIn) {
    return (
      <span className="auth-bar">
        <a href="/login">P01 ログイン</a>
      </span>
    );
  }
  return (
    <span className="auth-bar" style={{ display: "inline-flex", gap: "0.75rem", alignItems: "center" }}>
      {switcher}
      <span className="muted">{session.displayName ?? session.sub}</span>
      {" · "}
      <a href="/logout">ログアウト</a>
    </span>
  );
}
