export const apiBase = process.env.NEXT_PUBLIC_COMMERCE_API_URL ?? "http://localhost:8098";

export type Product = {
  id: string;
  sku: string;
  name: string;
  description: string;
  priceMinor: number;
  currency: string;
  imageUrl: string;
  availableQty: number;
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

export async function getProduct(id: string): Promise<Product> {
  const res = await fetch(`${apiBase}/v1/products/${id}`, { cache: "no-store" });
  if (!res.ok) throw new Error("product failed");
  return res.json();
}

export async function checkout(user: string, productId: string, qty: number, key: string) {
  const res = await fetch(`${apiBase}/v1/checkout`, {
    method: "POST",
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
