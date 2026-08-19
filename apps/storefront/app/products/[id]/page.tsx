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
      {p.reviews?.length ? (
        <section>
          <h2>レビュー</h2>
          <ul>
            {p.reviews.map((r) => (
              <li key={r.id}>
                <strong>{r.author}</strong>: {r.body}
              </li>
            ))}
          </ul>
        </section>
      ) : null}
      {p.imageUrl ? (
        // URL string only (P03 later). Demo hosts may 404; that is fine.
        // eslint-disable-next-line @next/next/no-img-element
        <img src={p.imageUrl} alt="" width={200} height={200} style={{ background: "#eee" }} />
      ) : null}
      <BuyBox productId={p.id} />
      {p.similar?.products?.length ? (
        <section>
          <h2>似ている商品</h2>
          <p style={{ color: "#666" }}>source: {p.similar.source}{p.similar.fallback ? " (fallback)" : ""}</p>
          <ul>
            {p.similar.products.map((row) => (
              <li key={row.id}>
                <a href={`/products/${row.id}`}>
                  {row.name} ({row.sku}) · {yen(row.priceMinor)}
                </a>
              </li>
            ))}
          </ul>
        </section>
      ) : null}
    </main>
  );
}
