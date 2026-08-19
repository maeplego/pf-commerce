# pf-commerce

P06 commerce-platform の製品リポジトリです。**学習用であり、本番 EC / 決済基盤の置き換えではありません。** 本物のカード番号は扱いません。

スライス 4–6 も **同一リポジトリのプロセス分割** です。8 個の git リポジトリにはしていません。公開 REST は gateway（`apps/api`）。購入者 GraphQL は BFF（`apps/bff`）。連携デモの overlay D は **catalog / inventory / order / gateway / storefront のみ**（payment / notify / bff / ops-web は Compose）。

## 構成

| パス | 役割 | Compose サービス |
| --- | --- | --- |
| `apps/catalog` | 商品マスタ + レビュー | `commerce-catalog` |
| `apps/inventory` | 在庫・引当・SSE | `commerce-inventory` |
| `apps/order` | チェックアウト、イベントストア、outbox | `commerce-order` |
| `apps/payment` | 決済モック（カードなし） | `commerce-payment` |
| `apps/notify` | 通知ログ（SMTP なし） | `commerce-notify` |
| `apps/api` | 公開 REST gateway。カート | `commerce-api` |
| `apps/bff` | GraphQL BFF + DataLoader | `commerce-bff` |
| `apps/storefront` | Next.js 購入者 | `commerce-storefront` |
| `apps/ops-web` | Next.js 在庫グリッド | `commerce-ops-web` |
| `deploy/` | Postgres 1 台 + 4 DB（catalog / inventory / orders / gateway） | |

全部を分割する必要はない、というのがこの構成の理由です。在庫と注文はライフサイクルが違うのでプロセスを分けました。決済と通知は注文に戻さず、失敗時の補償はイベントです。カートは gateway のままです。

K8s overlay D（P01+P02+P03+P06）は `deploy/k8s/` の既存 5 ワークロード。新サービスは overlay スクリプト側が未対応のため kustomization に入れていません。

## 単体デモ

```powershell
cd deploy
copy .env.example .env
docker compose up -d --build
```

| URL | 用途 |
| --- | --- |
| http://localhost:3009 | ストアフロント |
| http://localhost:3009/demo | 在庫 1 を buyer-a / buyer-b が同時購入 |
| http://localhost:3010 | ops-web（在庫グリッド + SSE） |
| http://localhost:8099/health | gateway liveness |
| http://localhost:8110/graphql | BFF |

### 在庫不足デモ

1. `/demo` を開く
2. 左右の「MUG-1 を 1点買う」をほぼ同時に押す
3. 片方 `paid`、もう片方 `inventory_shortage`（引当を戻す）

## テスト

```powershell
cd <repo root>
go test ./...
cd apps/bff
npm test
```

メモリ Store + httptest。BFF は DataLoader あり（REST 1 回）となし（N+1）を比較する。

## 既知の制限

- 8 サービス git ではない。cart は gateway
- overlay D は P06 フル + P07（payment / notify / bff / ops-web 搭載）
- Redis / RabbitMQ / 実メールなし。SSE はプロセス内 hub、outbox はポーリング
- P07 推薦は BFF fail-closed（カタログ順）。P03 実画像は未接続
- 予約 TTL 切れは次の Reserve 時に回収

設計: `project/portfolio-plan/commerce-platform/DESIGN.md`  
人間向け書類: `project/portfolio-plan/commerce-platform/docs/`
