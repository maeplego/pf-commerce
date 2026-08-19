# pf-commerce

P06 commerce-platform の製品リポジトリです。**学習用であり、本番 EC / 決済基盤の置き換えではありません。** 本物のカード番号は扱いません。

スライス 2 は **同一リポジトリのプロセス分割** です。8 個の git リポジトリにはしていません。公開入口は gateway（`apps/api`）。storefront は従来どおり `http://localhost:8099` だけを見ます。連携デモでは overlay D（`commerce.localhost` / `commerce-api.localhost`）。

## 構成

| パス | 役割 | Compose サービス |
| --- | --- | --- |
| `apps/catalog` | 商品マスタ | `commerce-catalog` |
| `apps/inventory` | 在庫・引当（UPDATE と reservation を 1 TX） | `commerce-inventory` |
| `apps/order` | チェックアウト。決済は order 内モック | `commerce-order` |
| `apps/api` | 公開 gateway。カートと商品+在庫の集約 | `commerce-api` |
| `apps/storefront` | Next.js | `commerce-storefront` |
| `deploy/` | Postgres 1 台 + 4 DB（catalog / inventory / orders / gateway） | |

共有は `packages/` の薄いもの（整数金額、ULID、dev auth、JSON）だけです。注文フローの正しさはプロセス間 HTTP でも、公開契約（在庫 1 → 201 と 409）は変えていません。

K8s overlay D（P01+P02+P03+P06）は `deploy/k8s/`。P07 / GraphQL BFF / 注文のイベントストア化は **未着手**。商品画像は URL 文字列。認証は `X-Dev-User-Sub`。

## 単体デモ

```powershell
cd deploy
copy .env.example .env
docker compose up -d --build
```

以前モノリス用の volume があるときは `docker compose down -v` してから上げてください（init で DB を分けるため）。

| URL | 用途 |
| --- | --- |
| http://localhost:3009 | ストアフロント |
| http://localhost:3009/demo | 在庫 1 を buyer-a / buyer-b が同時購入 |
| http://localhost:8099/health | gateway liveness |
| http://localhost:8099/ready | gateway + catalog/inventory/order |

catalog / inventory / order のポートは Compose ネットワーク内のみ（公開しない）。

連携デモ（Docker Desktop Kubernetes。overlay F など他 overlay と同時には載せない）:

```powershell
cd ..\pf-cloud-k8s
.\scripts\cluster-smoke-d-commerce.ps1
```

| URL | 用途 |
| --- | --- |
| http://commerce.localhost | ストアフロント |
| http://commerce.localhost/demo | 同時購入デモ |
| http://commerce-api.localhost/health | gateway |

シード: `MUG-1` 在庫 1、`TEE-1` 在庫 20、`STK-1` 在庫 0。金額は整数円。

### 在庫不足デモ

1. `/demo` を開く
2. 左右の「MUG-1 を 1点買う」をほぼ同時に押す
3. 片方 `paid`、もう片方 `inventory_shortage`（引当を戻す）

```powershell
curl http://localhost:8099/v1/products
curl -H "X-Dev-User-Sub: buyer-a" -H "Content-Type: application/json" `
  -d '{"idempotencyKey":"key-a","lines":[{"productId":"<MUG-ID>","qty":1}]}' `
  http://localhost:8099/v1/checkout
```

## テスト

```powershell
cd <repo root>
go test ./...
```

メモリ Store + httptest。gateway の HTTP テストは catalog / inventory / order を別 httptest サーバとして繋ぐ。

## 既知の制限

- まだ 8 サービス / 8 リポジトリではない。cart は gateway、決済は order 内モック
- overlay D は P06 サブセット。P07 / P11 / P12 / P13 は未搭載
- 予約 TTL 切れは次の Reserve 時に回収
- ops-web / GraphQL / 出荷 / メール / 推薦スロットなし

設計: `project/portfolio-plan/commerce-platform/DESIGN.md`  
人間向け書類: `project/portfolio-plan/commerce-platform/docs/`
