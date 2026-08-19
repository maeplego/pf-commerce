"use client";

import { useEffect, useState } from "react";
import { checkout, listProducts, type Product } from "@/lib/api";

function Pane({ user, mug }: { user: string; mug: Product | undefined }) {
  const [msg, setMsg] = useState("");
  async function buy() {
    if (!mug) return;
    setMsg("sending...");
    const { status, body } = await checkout(user, mug.id, 1, crypto.randomUUID());
    if (body.order) {
      setMsg(`${status} ${body.error?.code ?? ""} order=${body.order.status}`);
      return;
    }
    setMsg(`${status} ${body.status} ${body.id ?? ""}`);
  }
  return (
    <div style={{ flex: 1, border: "1px solid #ddd", padding: "1rem", borderRadius: 8 }}>
      <h2>{user}</h2>
      <button type="button" onClick={buy} disabled={!mug}>
        MUG-1 を 1点買う
      </button>
      <pre style={{ whiteSpace: "pre-wrap" }}>{msg}</pre>
    </div>
  );
}

export default function DemoPage() {
  const [mug, setMug] = useState<Product>();
  useEffect(() => {
    listProducts().then((ps) => setMug(ps.find((p) => p.sku === "MUG-1")));
  }, []);
  return (
    <main>
      <h1>在庫 1 の同時購入</h1>
      <p>
        Demo Mug の在庫は 1 です。左右をほぼ同時に押すと、片方だけ <code>paid</code>、もう片方は{" "}
        <code>inventory_shortage</code> で補償（引当解除）されます。カード番号は送りません。
      </p>
      <p>現在の表示在庫: {mug ? mug.availableQty : "..."}</p>
      <div style={{ display: "flex", gap: "1rem" }}>
        <Pane user="buyer-a" mug={mug} />
        <Pane user="buyer-b" mug={mug} />
      </div>
    </main>
  );
}
