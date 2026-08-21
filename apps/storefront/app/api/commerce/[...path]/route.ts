import { NextRequest, NextResponse } from "next/server";

import { readCookie } from "@/lib/oidc/cookies";
import { commerceApiBase, oidcEnabled } from "@/lib/oidc/env";
import { getCommerceSession } from "@/lib/session";

async function proxy(req: NextRequest, path: string[]) {
  const suffix = path.join("/");
  const url = new URL(`${commerceApiBase()}/${suffix}`);
  url.search = req.nextUrl.search;
  const headers = new Headers();
  const contentType = req.headers.get("content-type");
  if (contentType) {
    headers.set("content-type", contentType);
  }
  const idempotency = req.headers.get("idempotency-key");
  if (idempotency) {
    headers.set("Idempotency-Key", idempotency);
  }
  const session = await getCommerceSession();
  if (session.orgId) {
    headers.set("X-Dev-User-Org", session.orgId);
  }
  if (oidcEnabled()) {
    const access = await readCookie("rp_access");
    if (!access) {
      return NextResponse.json({ error: { code: "unauthorized", message: "missing credentials" } }, { status: 401 });
    }
    headers.set("Authorization", `Bearer ${access}`);
    headers.set("X-Dev-Role", "buyer");
  } else {
    const sub = req.headers.get("x-dev-user-sub") || session.sub;
    if (sub) {
      headers.set("X-Dev-User-Sub", sub);
      headers.set("X-Dev-Role", req.headers.get("x-dev-role") ?? "buyer");
    }
  }
  const init: RequestInit = { method: req.method, headers, cache: "no-store" };
  if (req.method !== "GET" && req.method !== "HEAD") {
    init.body = await req.arrayBuffer();
  }
  const upstream = await fetch(url, init);
  const body = await upstream.arrayBuffer();
  return new NextResponse(body, {
    status: upstream.status,
    headers: { "content-type": upstream.headers.get("content-type") ?? "application/json" },
  });
}

type Ctx = { params: Promise<{ path: string[] }> };

export async function GET(req: NextRequest, ctx: Ctx) {
  return proxy(req, (await ctx.params).path);
}

export async function POST(req: NextRequest, ctx: Ctx) {
  return proxy(req, (await ctx.params).path);
}

export async function PUT(req: NextRequest, ctx: Ctx) {
  return proxy(req, (await ctx.params).path);
}

export async function PATCH(req: NextRequest, ctx: Ctx) {
  return proxy(req, (await ctx.params).path);
}

export async function DELETE(req: NextRequest, ctx: Ctx) {
  return proxy(req, (await ctx.params).path);
}
