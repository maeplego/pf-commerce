"use client";

import { useEffect, useMemo, useState } from "react";

const apiBase = process.env.NEXT_PUBLIC_COMMERCE_API_URL ?? "http://localhost:8099";

type Row = {
  productId: string;
  sku: string;
  name: string;
  qty: number;
  reservedQty: number;
  availableQty: number;
};

type Mail = { id: string; type: string; orderId: string; buyerSub: string };

export default function OpsPage() {
  const [rows, setRows] = useState<Row[]>([]);
  const [feed, setFeed] = useState<string[]>([]);
  const [mail, setMail] = useState<Mail[]>([]);
  const [qty, setQty] = useState(1);
  const [err, setErr] = useState("");
  const headers = useMemo(
    () => ({ "Content-Type": "application/json", "X-Dev-User-Sub": "ops-demo", "X-Dev-Role": "ops" }),
    [],
  );

  async function load() {
    const res = await fetch(`${apiBase}/v1/ops/stock`, { headers, cache: "no-store" });
    if (!res.ok) throw new Error("stock failed");
    const body = await res.json();
    setRows(body.items as Row[]);
    const nres = await fetch(`${apiBase}/v1/ops/notifications`, { headers, cache: "no-store" });
    if (nres.ok) {
      const nbody = await nres.json();
      setMail(nbody.notifications as Mail[]);
    }
  }

  useEffect(() => {
    load().catch((e) => setErr(String(e)));
    const src = new EventSource(
      `${apiBase}/v1/ops/stock/stream?devUser=ops-demo&devRole=ops`,
    );
    src.addEventListener("stock.updated", (ev) => {
      const data = JSON.parse((ev as MessageEvent).data) as {
        productId: string;
        availableQty: number;
        qty: number;
        reservedQty: number;
        reason: string;
      };
      setFeed((prev) => [`${data.reason} ${data.productId} avail=${data.availableQty}`, ...prev].slice(0, 20));
      setRows((prev) =>
        prev.map((r) =>
          r.productId === data.productId
            ? { ...r, availableQty: data.availableQty, qty: data.qty, reservedQty: data.reservedQty }
            : r,
        ),
      );
    });
    return () => src.close();
  }, []);

  async function inbound(productId: string) {
    const res = await fetch(`${apiBase}/v1/ops/stock-inbound`, {
      method: "POST",
      headers,
      body: JSON.stringify({ productId, qty, reason: "ops-web" }),
    });
    if (!res.ok) {
      setErr(await res.text());
      return;
    }
    await load();
  }

  return (
    <main>
      <p style={{ color: "#555" }}>
        入庫するとグリッドとライブフィードが更新されます。認証は開発ヘッダ（ops）。Redis は使わずプロセス内 SSE です。
      </p>
      {err ? <p>{err}</p> : null}
      <div style={{ display: "grid", gridTemplateColumns: "2fr 1fr", gap: "1.5rem" }}>
        <section>
          <h1>在庫グリッド</h1>
          <label>
            入庫数量{" "}
            <input type="number" min={1} value={qty} onChange={(e) => setQty(Number(e.target.value))} />
          </label>
          <table style={{ width: "100%", borderCollapse: "collapse", marginTop: "0.75rem" }}>
            <thead>
              <tr>
                <th align="left">SKU</th>
                <th align="right">on-hand</th>
                <th align="right">reserved</th>
                <th align="right">available</th>
                <th></th>
              </tr>
            </thead>
            <tbody>
              {rows.map((r) => (
                <tr key={r.productId} style={{ background: r.availableQty <= 1 ? "#fff3cd" : undefined }}>
                  <td>
                    {r.sku} {r.name}
                  </td>
                  <td align="right">{r.qty}</td>
                  <td align="right">{r.reservedQty}</td>
                  <td align="right">{r.availableQty}</td>
                  <td>
                    <button type="button" onClick={() => inbound(r.productId)}>
                      入庫
                    </button>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </section>
        <section>
          <h2>ライブフィード</h2>
          <ul>
            {feed.map((line, i) => (
              <li key={i}>{line}</li>
            ))}
          </ul>
          <h2>通知 outbox</h2>
          <ul>
            {mail.map((m) => (
              <li key={m.id}>
                {m.type} {m.orderId.slice(0, 8)}…
              </li>
            ))}
          </ul>
        </section>
      </div>
    </main>
  );
}
