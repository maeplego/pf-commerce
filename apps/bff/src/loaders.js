export function createLoaders({ catalogURL, inventoryURL, siteId, fetchImpl, useLoader }) {
  const fetchFn = fetchImpl;
  const stats = { inventoryCalls: 0, reviewCalls: 0 };

  async function loadInventory(ids) {
    stats.inventoryCalls += 1;
    const q = `${inventoryURL}/v1/available?siteId=${encodeURIComponent(siteId)}&productIds=${ids.join(",")}`;
    const res = await fetchFn(q);
    const body = await res.json();
    const avail = body.available ?? {};
    return ids.map((id) => ({ availableQty: avail[id] ?? 0 }));
  }

  async function loadReviews(ids) {
    stats.reviewCalls += 1;
    const q = `${catalogURL}/v1/reviews?productIds=${ids.join(",")}`;
    const res = await fetchFn(q);
    const body = await res.json();
    const all = body.reviews ?? [];
    return ids.map((id) => all.filter((r) => r.productId === id));
  }

  if (!useLoader) {
    return {
      stats,
      inventory: { load: (id) => loadInventory([id]).then((rows) => rows[0]) },
      reviews: { load: (id) => loadReviews([id]).then((rows) => rows[0]) },
    };
  }

  return {
    stats,
    inventory: new Batcher(loadInventory),
    reviews: new Batcher(loadReviews),
  };
}

class Batcher {
  constructor(fn) {
    this.fn = fn;
    this.keys = [];
    this.resolvers = [];
    this.scheduled = false;
  }

  load(key) {
    return new Promise((resolve, reject) => {
      this.keys.push(key);
      this.resolvers.push({ resolve, reject });
      if (!this.scheduled) {
        this.scheduled = true;
        queueMicrotask(() => this.flush());
      }
    });
  }

  async flush() {
    const keys = this.keys;
    const resolvers = this.resolvers;
    this.keys = [];
    this.resolvers = [];
    this.scheduled = false;
    try {
      const rows = await this.fn(keys);
      rows.forEach((row, i) => resolvers[i].resolve(row));
    } catch (err) {
      resolvers.forEach((r) => r.reject(err));
    }
  }
}
