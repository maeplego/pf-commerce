/**
 * After `docker compose up`, wait until URLs return HTTP 2xx.
 * Does not start Compose. GitHub-hosted default CI does not run this
 * (image builds are too slow); see portfolio-plan/ci.md.
 */
const urls = process.argv.slice(2);
if (urls.length === 0) {
  console.error("usage: node scripts/compose-smoke.mjs URL [URL...]");
  process.exit(2);
}

const timeoutMs = Number(process.env.SMOKE_TIMEOUT_MS ?? 120000);
const intervalMs = 2000;
const started = Date.now();

async function once(url) {
  const res = await fetch(url, { redirect: "manual" });
  if (res.status >= 200 && res.status < 300) {
    return true;
  }
  throw new Error(`${url} -> ${res.status}`);
}

async function wait(url) {
  let last = "";
  while (Date.now() - started < timeoutMs) {
    try {
      await once(url);
      console.log("ok", url);
      return;
    } catch (err) {
      last = err instanceof Error ? err.message : String(err);
      await new Promise((r) => setTimeout(r, intervalMs));
    }
  }
  throw new Error(`timeout waiting for ${url}: ${last}`);
}

for (const url of urls) {
  await wait(url);
}
