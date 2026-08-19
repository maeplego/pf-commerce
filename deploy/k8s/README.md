# P06 commerce Kubernetes manifests

catalog / inventory / order / payment / notify / gateway / bff / storefront / ops-web. payment と notify はインメモリ（Postgres 不要）。BFF は `RECOMMEND_API_URL=http://api.p07.svc.cluster.local:8098`。P07 失敗時はカタログ順（人気の代替）。

overlay smoke は `COMMERCE_DEV_AUTH` + `X-Dev-User-Sub`。単体 apply ではなく `pf-cloud-k8s` overlay `d-commerce` から参照する。

Ingress（`pf-cloud-k8s`）:

| ホスト | Service | 用途 |
| --- | --- | --- |
| `commerce.localhost` | web:3009 | ストアフロント。`/demo` は在庫 1 の同時購入 |
| `commerce-api.localhost` | api:8099 | 公開 gateway（カート + checkout） |
| `commerce-bff.localhost` | bff:8110 | GraphQL BFF（おすすめ / 類似） |
| `commerce-ops.localhost` | ops-web:3010 | 在庫ダッシュボード |

catalog / inventory / order / payment / notify は Ingress しない（クラスタ内のみ）。

Postgres は platform の DB 名 `commerce_catalog` / `commerce_inventory` / `commerce_order` / `commerce_gateway`。

```powershell
cd ..\..\pf-cloud-k8s
.\scripts\cluster-smoke-d-commerce.ps1
```
