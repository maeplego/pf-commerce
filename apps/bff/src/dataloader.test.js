import assert from "node:assert/strict";
import { test } from "node:test";
import { createLoaders } from "./loaders.js";
import { executeQuery } from "./server.js";

function fakeRest() {
  const products = [
    { id: "p1", sku: "MUG-1", name: "Mug", description: "", priceMinor: 1200, currency: "JPY", imageUrl: "" },
    { id: "p2", sku: "TEE-1", name: "Tee", description: "", priceMinor: 3500, currency: "JPY", imageUrl: "" },
    { id: "p3", sku: "STK-1", name: "Sticker", description: "", priceMinor: 300, currency: "JPY", imageUrl: "" },
  ];
  const calls = { inventory: 0, reviews: 0 };
  const fetchImpl = async (url) => {
    const u = String(url);
    if (u.includes("/v1/products") && !u.includes("/v1/products/")) {
      return { status: 200, json: async () => ({ products }) };
    }
    if (u.includes("/v1/available")) {
      calls.inventory += 1;
      return { status: 200, json: async () => ({ available: { p1: 1, p2: 20, p3: 0 } }) };
    }
    if (u.includes("/v1/reviews")) {
      calls.reviews += 1;
      return {
        status: 200,
        json: async () => ({
          reviews: [
            { id: "r1", productId: "p1", author: "a", body: "nice" },
            { id: "r2", productId: "p1", author: "b", body: "ok" },
          ],
        }),
      };
    }
    throw new Error(u);
  };
  return { fetchImpl, calls };
}

const query = `{ products { id inventory { availableQty } reviews { id body } } }`;

test("DataLoader batches inventory and reviews (one REST call each)", async () => {
  const { fetchImpl, calls } = fakeRest();
  const loaders = createLoaders({
    catalogURL: "http://catalog",
    inventoryURL: "http://inventory",
    siteId: "site",
    fetchImpl,
    useLoader: true,
  });
  const result = await executeQuery({ query, siteId: "site", fetchImpl, loaders });
  assert.equal(result.errors, undefined);
  assert.equal(result.data.products.length, 3);
  assert.equal(result.data.products[0].inventory.availableQty, 1);
  assert.equal(result.data.products[0].reviews.length, 2);
  assert.equal(calls.inventory, 1);
  assert.equal(calls.reviews, 1);
});

test("without DataLoader each product hits inventory REST (N+1)", async () => {
  const { fetchImpl, calls } = fakeRest();
  const loaders = createLoaders({
    catalogURL: "http://catalog",
    inventoryURL: "http://inventory",
    siteId: "site",
    fetchImpl,
    useLoader: false,
  });
  const result = await executeQuery({ query, siteId: "site", fetchImpl, loaders });
  assert.equal(result.errors, undefined);
  assert.equal(calls.inventory, 3);
  assert.equal(calls.reviews, 3);
});

test("recommended slot does not break DataLoader batching on products", async () => {
  const { fetchImpl, calls } = fakeRest();
  const loaders = createLoaders({
    catalogURL: "http://catalog",
    inventoryURL: "http://inventory",
    siteId: "site",
    fetchImpl,
    useLoader: true,
  });
  const result = await executeQuery({
    query: `{ products { id inventory { availableQty } } recommended(userId: "anon", k: 1) { source products { sku } } }`,
    siteId: "site",
    fetchImpl,
    loaders,
  });
  assert.equal(result.errors, undefined);
  assert.equal(result.data.recommended.source, "popularity");
  assert.equal(calls.inventory, 1);
});
