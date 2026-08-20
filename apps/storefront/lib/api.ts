export const apiBase = process.env.NEXT_PUBLIC_COMMERCE_API_URL ?? "http://localhost:8099";
export const bffBase = process.env.NEXT_PUBLIC_COMMERCE_BFF_URL ?? "";

export type Product = {
  id: string;
  sku: string;
  name: string;
  description: string;
  priceMinor: number;
  currency: string;
  imageUrl: string;
  availableQty: number;
  reviews?: { id: string; author: string; body: string }[];
  similar?: RecommendSlot;
};

export type RecommendSlot = {
  source: string;
  fallback: boolean;
  products: Product[];
};

export type Order = {
  id: string;
  buyerSub: string;
  status: string;
  cancelReason: string;
  amountMinor: number;
  currency: string;
  lines: { productId: string; sku: string; name: string; qty: number; unitPriceMinor: number }[];
};

export function yen(minor: number) {
  return `¥${minor.toLocaleString("ja-JP")}`;
}

export async function listProducts(): Promise<Product[]> {
  const res = await fetch(`${apiBase}/v1/products`, { cache: "no-store" });
  if (!res.ok) throw new Error("products failed");
  const body = await res.json();
  return body.products as Product[];
}

export async function listRecommended(userId: string): Promise<RecommendSlot | null> {
  if (!bffBase) return null;
  const res = await fetch(`${bffBase}/graphql`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    cache: "no-store",
    body: JSON.stringify({
      query: `query ($userId: ID!) { recommended(userId: $userId, k: 5) { source fallback products { id sku name priceMinor currency } } }`,
      variables: { userId },
    }),
  });
  if (!res.ok) return null;
  const body = await res.json();
  return body.data?.recommended ?? null;
}

export async function getProduct(id: string): Promise<Product> {
  if (bffBase) {
    const res = await fetch(`${bffBase}/graphql`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      cache: "no-store",
      body: JSON.stringify({
        query: `query ($id: ID!) { product(id: $id) { id sku name description priceMinor currency imageUrl inventory { availableQty } reviews { id author body } similar(k: 5) { source fallback products { id sku name priceMinor currency } } } }`,
        variables: { id },
      }),
    });
    if (!res.ok) throw new Error("bff failed");
    const body = await res.json();
    const p = body.data?.product;
    if (!p) throw new Error("product failed");
    return { ...p, availableQty: p.inventory?.availableQty ?? 0, reviews: p.reviews ?? [], similar: p.similar };
  }
  const res = await fetch(`${apiBase}/v1/products/${id}`, { cache: "no-store" });
  if (!res.ok) throw new Error("product failed");
  return res.json();
}

export async function checkout(user: string, productId: string, qty: number, key: string) {
  const res = await fetch(`/api/commerce/v1/checkout`, {
    method: "POST",
    credentials: "same-origin",
    headers: {
      "Content-Type": "application/json",
      "X-Dev-User-Sub": user,
      "X-Dev-Role": "buyer",
      "Idempotency-Key": key,
    },
    body: JSON.stringify({ idempotencyKey: key, lines: [{ productId, qty }] }),
  });
  const body = await res.json();
  return { status: res.status, body };
}
