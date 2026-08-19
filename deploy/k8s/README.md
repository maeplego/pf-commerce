# Kubernetes マニフェスト（P06 commerce）

catalog / inventory / order / payment / notify / gateway / bff / storefront / ops-web です。このフォルダだけを apply しないでください。起動は [pf-cloud-k8s](https://github.com/maeplego/pf-cloud-k8s) の commerce overlay からです。

| ホスト | 用途 |
| --- | --- |
| `commerce.localhost` | ストアフロント。`/demo` は在庫 1 の同時購入 |
| `commerce-api.localhost` | 公開 gateway |
| `commerce-bff.localhost` | GraphQL |
| `commerce-ops.localhost` | 在庫グリッド |

catalog などは Ingress しません。推薦 API が落ちると BFF はカタログ順に戻します。
