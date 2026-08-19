"use client";

import { useEffect, useState } from "react";
import { getProduct, yen, type Product } from "@/lib/api";
import { BuyBox } from "./BuyBox";

export default function ProductPage({ params }: { params: Promise<{ id: string }> }) {
  const [p, setP] = useState<Product>();
  const [err, setErr] = useState("");
  useEffect(() => {
    params.then(({ id }) => getProduct(id)).then(setP).catch((e) => setErr(String(e)));
  }, [params]);
  if (err) return <p>{err}</p>;
  if (!p) return <p>読み込み中…</p>;
  return (
    <main>
      <p>
        <a href="/">← カタログ</a>
      </p>
      <h1>{p.name}</h1>
      <p>
        {p.sku} · {yen(p.priceMinor)} · 在庫 {p.availableQty}
      </p>
      <p>{p.description}</p>
      {p.imageUrl ? (
        // URL string only (P03 later). Demo hosts may 404; that is fine.
        // eslint-disable-next-line @next/next/no-img-element
        <img src={p.imageUrl} alt="" width={200} height={200} style={{ background: "#eee" }} />
      ) : null}
      <BuyBox productId={p.id} />
    </main>
  );
}
