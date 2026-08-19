/**
 * P07 adapter for buyer GraphQL. Catalog has no tags; fail closed to catalog list order
 * (popularity stand-in). item_id is SKU because catalog ULIDs are seed-generated.
 */

export function popularityProducts(products, { excludeId, k } = {}) {
  const limit = k ?? 5;
  return products.filter((p) => p.id !== excludeId).slice(0, limit);
}

export function mapSkus(products, skus) {
  const bySku = new Map(products.map((p) => [p.sku, p]));
  const mapped = [];
  for (const sku of skus) {
    const row = bySku.get(sku);
    if (!row) return [];
    mapped.push(row);
  }
  return mapped;
}

export async function fetchRecommendItems({ url, fetchImpl, timeoutMs = 1500 }) {
  if (!url) return null;
  const fetchFn = fetchImpl ?? fetch;
  const ac = new AbortController();
  const timer = setTimeout(() => ac.abort(), timeoutMs);
  try {
    const res = await fetchFn(url, { signal: ac.signal });
    if (!res || !res.ok) return null;
    const body = await res.json();
    const items = body.items ?? [];
    if (!items.length) return null;
    return items.map((item) => item.item_id).filter(Boolean);
  } catch {
    return null;
  } finally {
    clearTimeout(timer);
  }
}

export async function resolveSlot({ products, excludeId, k, recommendUrl, fetchImpl, timeoutMs }) {
  const fallbackProducts = popularityProducts(products, { excludeId, k });
  const skus = await fetchRecommendItems({ url: recommendUrl, fetchImpl, timeoutMs });
  if (!skus) {
    return { source: "popularity", fallback: true, products: fallbackProducts };
  }
  const mapped = mapSkus(products, skus)
    .filter((p) => p.id !== excludeId)
    .slice(0, k ?? 5);
  if (!mapped.length) {
    return { source: "popularity", fallback: true, products: fallbackProducts };
  }
  return { source: "recommend", fallback: false, products: mapped };
}
