"use client";

import { useState } from "react";
import { checkout } from "@/lib/api";

export function BuyBox({ productId }: { productId: string }) {
  const [user, setUser] = useState("buyer-a");
  const [msg, setMsg] = useState("");

  async function onBuy() {
    setMsg("...");
    const key = crypto.randomUUID();
    const { status, body } = await checkout(user, productId, 1, key);
    if (status === 201 || status === 200) {
      setMsg(`${status} ${body.status} ${body.id}`);
      return;
    }
    setMsg(`${status} ${body.error?.code ?? ""} ${body.order?.status ?? ""}`);
  }

  return (
    <section style={{ marginTop: "1rem" }}>
      <label>
        購入者（dev auth）{" "}
        <select value={user} onChange={(e) => setUser(e.target.value)}>
          <option value="buyer-a">buyer-a</option>
          <option value="buyer-b">buyer-b</option>
        </select>
      </label>
      <div style={{ marginTop: "0.75rem" }}>
        <button type="button" onClick={onBuy}>
          1点をチェックアウト
        </button>
      </div>
      <pre style={{ marginTop: "0.75rem", whiteSpace: "pre-wrap" }}>{msg}</pre>
    </section>
  );
}
