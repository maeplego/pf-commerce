/**
 * Buyer GraphQL BFF. REST catalog/inventory stay the public contract.
 * DataLoader batches Product.inventory and Product.reviews (the N+1 path).
 * Recommend slots fail closed to catalog list order when P07 is missing or unmapped.
 */
import { createServer } from "node:http";
import { graphql, buildSchema } from "./graphql.js";
import { createLoaders } from "./loaders.js";
import { resolveSlot } from "./recommend.js";

const catalogURL = (process.env.COMMERCE_CATALOG_URL ?? "http://localhost:8101").replace(/\/$/, "");
const inventoryURL = (process.env.COMMERCE_INVENTORY_URL ?? "http://localhost:8102").replace(/\/$/, "");
const recommendApiURLEnv = (process.env.RECOMMEND_API_URL ?? "").replace(/\/$/, "");
const port = Number(process.env.COMMERCE_HTTP_PORT ?? "8110");
const useLoader = process.env.COMMERCE_BFF_DATALOADER !== "0";
const maxCost = 200;

export const schemaSDL = `
  type Inventory { availableQty: Int! }
  type Review { id: ID!, productId: ID!, author: String!, body: String! }
  type Product {
    id: ID!
    sku: String!
    name: String!
    description: String!
    priceMinor: Int!
    currency: String!
    imageUrl: String!
    inventory: Inventory
    reviews: [Review!]!
    similar(k: Int = 5): RecommendSlot!
  }
  type RecommendSlot { source: String!, fallback: Boolean!, products: [Product!]! }
  type Query {
    products: [Product!]!
    product(id: ID!): Product
    recommended(userId: ID!, k: Int = 5): RecommendSlot!
  }
`;

export function costOf(query) {
  const n = (query.match(/\b(inventory|reviews|products|product|similar|recommended)\b/g) ?? []).length;
  return n * 10 + query.length / 20;
}

async function loadCatalogProducts(fetchFn) {
  const res = await fetchFn(`${catalogURL}/v1/products`);
  const body = await res.json();
  return body.products ?? [];
}

export async function executeQuery({ query, variables, siteId, fetchImpl, loaders, recommendApiURL }) {
  if (costOf(query) > maxCost) {
    return { errors: [{ message: "query too expensive" }] };
  }
  const schema = buildSchema(schemaSDL);
  const fetchFn = fetchImpl ?? fetch;
  const recBase = (recommendApiURL ?? recommendApiURLEnv).replace(/\/$/, "");
  const L = loaders ?? createLoaders({ catalogURL, inventoryURL, siteId, fetchImpl: fetchFn, useLoader });
  const root = {
    products: async () => loadCatalogProducts(fetchFn),
    product: async ({ id }) => {
      const res = await fetchFn(`${catalogURL}/v1/products/${id}`);
      if (res.status === 404) return null;
      return res.json();
    },
    recommended: async ({ userId, k }) => {
      const products = await loadCatalogProducts(fetchFn);
      const limit = k ?? 5;
      const url = recBase
        ? `${recBase}/v1/recommend?namespace=commerce&user_id=${encodeURIComponent(userId)}&k=${limit}`
        : "";
      return resolveSlot({ products, k: limit, recommendUrl: url, fetchImpl: fetchFn });
    },
  };
  return graphql({
    schema,
    source: query,
    variableValues: variables,
    rootValue: root,
    fieldResolver: (source, args, ctx, info) => {
      if (info.parentType.name === "Query") {
        return root[info.fieldName](args);
      }
      if (info.fieldName === "inventory") {
        return L.inventory.load(source.id);
      }
      if (info.fieldName === "reviews") {
        return L.reviews.load(source.id);
      }
      if (info.fieldName === "similar") {
        const limit = args.k ?? 5;
        return loadCatalogProducts(fetchFn).then((products) => {
          const url = recBase
            ? `${recBase}/v1/similar-items?namespace=commerce&item_id=${encodeURIComponent(source.sku)}&k=${limit}`
            : "";
          return resolveSlot({
            products,
            excludeId: source.id,
            k: limit,
            recommendUrl: url,
            fetchImpl: fetchFn,
          });
        });
      }
      return source?.[info.fieldName];
    },
  });
}

const siteCache = { id: "" };

async function siteId() {
  if (siteCache.id) return siteCache.id;
  const res = await fetch(`${inventoryURL}/v1/sites/code/MAIN`);
  const body = await res.json();
  siteCache.id = body.id;
  return siteCache.id;
}

const server = createServer(async (req, res) => {
  if (req.url === "/health" || req.url === "/ready") {
    res.setHeader("Content-Type", "application/json");
    res.end(JSON.stringify({ ok: true }));
    return;
  }
  if (req.method === "POST" && req.url === "/graphql") {
    const chunks = [];
    for await (const c of req) chunks.push(c);
    let body = {};
    try {
      body = JSON.parse(Buffer.concat(chunks).toString("utf8") || "{}");
    } catch {
      res.statusCode = 400;
      res.end(JSON.stringify({ errors: [{ message: "invalid json" }] }));
      return;
    }
    const sid = await siteId();
    const result = await executeQuery({ query: body.query, variables: body.variables, siteId: sid });
    res.setHeader("Content-Type", "application/json");
    res.setHeader("Access-Control-Allow-Origin", "*");
    res.end(JSON.stringify(result));
    return;
  }
  if (req.method === "OPTIONS") {
    res.setHeader("Access-Control-Allow-Origin", "*");
    res.setHeader("Access-Control-Allow-Headers", "Content-Type");
    res.statusCode = 204;
    res.end();
    return;
  }
  res.statusCode = 404;
  res.end("not found");
});

if (process.argv[1] && process.argv[1].endsWith("server.js")) {
  server.listen(port, () => {
    console.log(`commerce bff listening on :${port} dataloader=${useLoader}`);
  });
}

export { server };
