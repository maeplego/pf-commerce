"use client";

import { useEffect, useState } from "react";
import { listProducts, listRecommended, yen, type Product, type RecommendSlot } from "@/lib/api";

export default function HomePage() {
  const [products, setProducts] = useState<Product[]>([]);
  const [slot, setSlot] = useState<RecommendSlot | null>(null);
  const [err, setErr] = useState("");
  useEffect(() => {
    listProducts()
      .then(setProducts)
      .catch((e) => setErr(String(e)));
    listRecommended("anon")
      .then(setSlot)
      .catch(() => setSlot(null));
  }, []);
  return (
    <main>
      <h1>カタログ</h1>
      <p style={{ color: "#555" }}>
        金額は整数（円）。画像は URL 文字列（P03 未接続）。決済はモックでカード番号は扱いません。
      </p>
      {err ? <p>{err}</p> : null}
      {slot?.products?.length ? (
        <section>
          <h2>おすすめ</h2>
          <p style={{ color: "#666" }}>
            source: {slot.source}
            {slot.fallback ? " (fallback)" : ""}
          </p>
          <ul style={{ listStyle: "none", padding: 0, display: "grid", gap: "1rem" }}>
            {slot.products.map((p) => (
              <li key={p.id} style={{ border: "1px solid #ddd", padding: "1rem", borderRadius: 8 }}>
                <a href={`/products/${p.id}`} style={{ color: "inherit", textDecoration: "none" }}>
                  <strong>{p.name}</strong> <span style={{ color: "#666" }}>{p.sku}</span>
                  <div>{yen(p.priceMinor)}</div>
                </a>
              </li>
            ))}
          </ul>
        </section>
      ) : null}
      <ul style={{ listStyle: "none", padding: 0, display: "grid", gap: "1rem" }}>
        {products.map((p) => (
          <li key={p.id} style={{ border: "1px solid #ddd", padding: "1rem", borderRadius: 8 }}>
            <a href={`/products/${p.id}`} style={{ color: "inherit", textDecoration: "none" }}>
              <strong>{p.name}</strong> <span style={{ color: "#666" }}>{p.sku}</span>
              <div>
                {yen(p.priceMinor)} · 在庫 {p.availableQty}
              </div>
              <p style={{ margin: "0.5rem 0 0" }}>{p.description}</p>
            </a>
          </li>
        ))}
      </ul>
    </main>
  );
}
