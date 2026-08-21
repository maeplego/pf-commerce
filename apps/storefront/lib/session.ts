import { readCookie } from "./oidc/cookies";
import { internalBase, oidcEnabled } from "./oidc/env";

export type OrgMembership = {
  orgId: string;
  orgName: string;
  role: string;
};

export type CommerceSession = {
  sub: string | null;
  displayName: string | null;
  accessToken?: string;
  orgId: string;
  organizations: OrgMembership[];
  oidc: boolean;
  loggedIn: boolean;
  devMode: boolean;
};

const DEMO_ORGS: OrgMembership[] = [
  { orgId: "org-demo-a", orgName: "Demo Org A", role: "owner" },
  { orgId: "org-demo-b", orgName: "Demo Org B", role: "member" },
];

export async function getCommerceSession(devUser?: string): Promise<CommerceSession> {
  if (!oidcEnabled()) {
    const saved = (await readCookie("dev_org")) || "";
    const orgId = saved || DEMO_ORGS[0].orgId;
    return {
      sub: devUser?.trim() || "buyer-a",
      displayName: null,
      orgId,
      organizations: DEMO_ORGS,
      oidc: false,
      loggedIn: true,
      devMode: true,
    };
  }
  const access = await readCookie("rp_access");
  if (!access) {
    return {
      sub: null,
      displayName: null,
      orgId: DEMO_ORGS[0].orgId,
      organizations: [],
      oidc: true,
      loggedIn: false,
      devMode: false,
    };
  }
  const res = await fetch(`${internalBase()}/userinfo`, {
    headers: { Authorization: `Bearer ${access}` },
    cache: "no-store",
  });
  if (!res.ok) {
    return {
      sub: null,
      displayName: null,
      orgId: DEMO_ORGS[0].orgId,
      organizations: [],
      oidc: true,
      loggedIn: false,
      devMode: false,
    };
  }
  const ui = (await res.json()) as {
    sub?: string;
    name?: string;
    email?: string;
    org_id?: string;
    organizations?: { org_id?: string; org_name?: string; role?: string }[];
  };
  if (!ui.sub) {
    return {
      sub: null,
      displayName: null,
      orgId: DEMO_ORGS[0].orgId,
      organizations: [],
      oidc: true,
      loggedIn: false,
      devMode: false,
    };
  }
  const organizations = (ui.organizations || [])
    .filter((o) => o.org_id)
    .map((o) => ({
      orgId: String(o.org_id),
      orgName: String(o.org_name || o.org_id),
      role: String(o.role || "member"),
    }));
  return {
    sub: ui.sub,
    displayName: ui.name || ui.email || ui.sub,
    accessToken: access,
    orgId: ui.org_id ? String(ui.org_id) : organizations[0]?.orgId || DEMO_ORGS[0].orgId,
    organizations,
    oidc: true,
    loggedIn: true,
    devMode: false,
  };
}
