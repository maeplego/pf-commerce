# P06 commerce Kubernetes manifests

catalog / inventory / order / gateway / storefront。overlay smoke は `COMMERCE_DEV_AUTH` + `X-Dev-User-Sub`。単体 apply ではなく `pf-cloud-k8s` overlay `d-commerce` から参照する。

Ingress（`pf-cloud-k8s`）:

| ホスト | Service | 用途 |
| --- | --- | --- |
| `commerce.localhost` | web:3009 | ストアフロント。`/demo` は在庫 1 の同時購入 |
| `commerce-api.localhost` | api:8099 | 公開 gateway（カート + checkout） |

catalog / inventory / order は Ingress しない（クラスタ内のみ）。

Postgres は platform の DB 名 `commerce_catalog` / `commerce_inventory` / `commerce_order` / `commerce_gateway`。

```powershell
cd ..\..\pf-cloud-k8s
.\scripts\cluster-smoke-d-commerce.ps1
```
