import assert from "node:assert/strict";
import { test } from "node:test";
import { popularityProducts, mapSkus, fetchRecommendItems, resolveSlot } from "./recommend.js";
import { executeQuery } from "./server.js";

const products = [
  { id: "p1", sku: "MUG-1", name: "Mug", description: "", priceMinor: 1200, currency: "JPY", imageUrl: "" },
  { id: "p2", sku: "TEE-1", name: "Tee", description: "", priceMinor: 3500, currency: "JPY", imageUrl: "" },
  { id: "p3", sku: "STK-1", name: "Sticker", description: "", priceMinor: 300, currency: "JPY", imageUrl: "" },
];

test("popularityProducts keeps catalog order and excludes id", () => {
  const rows = popularityProducts(products, { excludeId: "p1", k: 2 });
  assert.deepEqual(
    rows.map((p) => p.sku),
    ["TEE-1", "STK-1"],
  );
});

test("mapSkus maps P07 item_id SKUs; unknown SKUs yield empty", () => {
  assert.deepEqual(
    mapSkus(products, ["TEE-1", "STK-1"]).map((p) => p.id),
    ["p2", "p3"],
  );
  assert.deepEqual(mapSkus(products, ["TEE-1", "NOPE"]), []);
});

test("fetchRecommendItems returns null on 500, throw, and timeout", async () => {
  const down = await fetchRecommendItems({
    url: "http://recommend/v1/similar-items",
    fetchImpl: async () => {
      throw new Error("ECONNREFUSED");
    },
  });
  assert.equal(down, null);

  const bad = await fetchRecommendItems({
    url: "http://recommend/v1/similar-items",
    fetchImpl: async () => ({ ok: false, status: 500, json: async () => ({}) }),
  });
  assert.equal(bad, null);

  const slow = await fetchRecommendItems({
    url: "http://recommend/v1/similar-items",
    timeoutMs: 20,
    fetchImpl: (_url, init) =>
      new Promise((resolve, reject) => {
        init?.signal?.addEventListener("abort", () => reject(Object.assign(new Error("aborted"), { name: "AbortError" })));
      }),
  });
  assert.equal(slow, null);
});

test("resolveSlot uses popularity when P07 is down", async () => {
  const slot = await resolveSlot({
    products,
    excludeId: "p1",
    k: 2,
    recommendUrl: "http://recommend/v1/similar-items",
    fetchImpl: async () => {
      throw new Error("down");
    },
  });
  assert.equal(slot.source, "popularity");
  assert.equal(slot.fallback, true);
  assert.deepEqual(
    slot.products.map((p) => p.sku),
    ["TEE-1", "STK-1"],
  );
});

test("resolveSlot maps SKUs from P07", async () => {
  const slot = await resolveSlot({
    products,
    k: 2,
    recommendUrl: "http://recommend/v1/recommend",
    fetchImpl: async () => ({
      ok: true,
      json: async () => ({ items: [{ item_id: "STK-1" }, { item_id: "TEE-1" }] }),
    }),
  });
  assert.equal(slot.source, "recommend");
  assert.equal(slot.fallback, false);
  assert.deepEqual(
    slot.products.map((p) => p.sku),
    ["STK-1", "TEE-1"],
  );
});

test("resolveSlot unknown SKUs fail closed to popularity", async () => {
  const slot = await resolveSlot({
    products,
    k: 2,
    recommendUrl: "http://recommend/v1/recommend",
    fetchImpl: async () => ({
      ok: true,
      json: async () => ({ items: [{ item_id: "UNKNOWN" }] }),
    }),
  });
  assert.equal(slot.source, "popularity");
  assert.equal(slot.fallback, true);
  assert.deepEqual(
    slot.products.map((p) => p.sku),
    ["MUG-1", "TEE-1"],
  );
});

function catalogFetch(p07) {
  return async (url) => {
    const u = String(url);
    if (u.includes("/v1/products") && !u.includes("/v1/products/")) {
      return { status: 200, ok: true, json: async () => ({ products }) };
    }
    if (u.includes("/v1/products/")) {
      return { status: 200, ok: true, json: async () => products[0] };
    }
    if (u.includes("/v1/available")) {
      return { status: 200, ok: true, json: async () => ({ available: { p1: 1, p2: 20, p3: 0 } }) };
    }
    if (u.includes("/v1/reviews")) {
      return { status: 200, ok: true, json: async () => ({ reviews: [] }) };
    }
    if (u.includes("/v1/similar-items") || u.includes("/v1/recommend")) {
      return p07(u);
    }
    throw new Error(u);
  };
}

test("GraphQL similar / recommended fail closed when P07 is empty", async () => {
  const fetchImpl = catalogFetch(async () => ({ ok: false, status: 500, json: async () => ({}) }));
  const similar = await executeQuery({
    query: `{ product(id: "p1") { sku similar(k: 2) { source fallback products { sku } } } }`,
    fetchImpl,
    recommendApiURL: "http://recommend",
  });
  assert.equal(similar.errors, undefined);
  assert.equal(similar.data.product.similar.source, "popularity");
  assert.equal(similar.data.product.similar.fallback, true);
  assert.deepEqual(
    similar.data.product.similar.products.map((p) => p.sku),
    ["TEE-1", "STK-1"],
  );

  const rec = await executeQuery({
    query: `{ recommended(userId: "anon", k: 2) { source fallback products { sku } } }`,
    fetchImpl,
    recommendApiURL: "http://recommend",
  });
  assert.equal(rec.errors, undefined);
  assert.equal(rec.data.recommended.source, "popularity");
});

test("GraphQL similar maps SKU from P07", async () => {
  const fetchImpl = catalogFetch(async (u) => {
    assert.match(u, /namespace=commerce/);
    assert.match(u, /item_id=MUG-1/);
    return { ok: true, json: async () => ({ items: [{ item_id: "TEE-1" }] }) };
  });
  const result = await executeQuery({
    query: `{ product(id: "p1") { similar(k: 1) { source products { sku } } } }`,
    fetchImpl,
    recommendApiURL: "http://recommend",
  });
  assert.equal(result.errors, undefined);
  assert.equal(result.data.product.similar.source, "recommend");
  assert.equal(result.data.product.similar.products[0].sku, "TEE-1");
});
