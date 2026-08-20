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
    <>
      <section className="hero">
        <h1 className="page-title">カタログ</h1>
        <p className="page-lead">
          金額は整数（円）。画像は URL 文字列（P03 未接続）。決済はモックでカード番号は扱いません。
        </p>
      </section>
      {err ? <p className="error">{err}</p> : null}
      {slot?.products?.length ? (
        <section>
          <h2 className="section-title">おすすめ</h2>
          <p className="muted">
            source: {slot.source}
            {slot.fallback ? " (fallback)" : ""}
          </p>
          <div className="card-grid">
            {slot.products.map((p) => (
              <article key={p.id} className="card">
                <a href={`/products/${p.id}`} className="product-link">
                  <strong>{p.name}</strong> <span className="muted">{p.sku}</span>
                  <div>{yen(p.priceMinor)}</div>
                </a>
              </article>
            ))}
          </div>
        </section>
      ) : null}
      <div className="card-grid">
        {products.map((p) => (
          <article key={p.id} className="card">
            <a href={`/products/${p.id}`} className="product-link">
              <strong>{p.name}</strong> <span className="muted">{p.sku}</span>
              <div>
                {yen(p.priceMinor)} · 在庫 {p.availableQty}
              </div>
              <p className="muted">{p.description}</p>
            </a>
          </article>
        ))}
      </div>
    </>
  );
}
