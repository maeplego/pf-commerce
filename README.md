# pf-commerce

学習用の EC です。商品、在庫引当、注文、決済モック、通知ログ、公開 REST、GraphQL BFF、ストアフロント、在庫グリッドを **1 リポジトリのプロセス分割** で動かします。本物のカード番号は扱いません。**本番 EC の置き換えではありません。**

在庫と注文はライフサイクルが違うのでプロセスを分けています。カートは gateway に残しています。

| ディレクトリ | 役割 |
| --- | --- |
| `apps/catalog` | 商品とレビュー |
| `apps/inventory` | 在庫・引当・SSE |
| `apps/order` | チェックアウトと outbox |
| `apps/payment` | 決済モック |
| `apps/notify` | 通知ログ（実メールなし） |
| `apps/api` | 公開 REST（カート含む） |
| `apps/bff` | GraphQL + DataLoader |
| `apps/storefront` | 購入者 UI |
| `apps/ops-web` | 在庫グリッド |
| `deploy/` | Postgres（catalog / inventory / orders / gateway） |

## 起動

```powershell
cd deploy
copy .env.example .env
docker compose up -d --build
```

| URL | 用途 |
| --- | --- |
| http://localhost:3009 | ストアフロント |
| http://localhost:3009/demo | 在庫 1 を 2 人が同時に買う |
| http://localhost:3010 | 在庫グリッド |
| http://localhost:8099/health | gateway |
| http://localhost:8110/health | BFF |

BFF の GraphQL はストアフロント origin（既定 `http://localhost:3009`）からのブラウザ呼び出しだけ CORS を付けます。curl など Origin なしは通します。

`/demo` で左右の「MUG-1 を 1点買う」をほぼ同時に押すと、片方だけ成功し、もう片方は在庫不足になります。

推薦 API が落ちているときは、BFF はカタログ順に戻します（fail-closed）。

## テスト

```powershell
go test ./...
cd apps/bff
npm test
```

`/demo` のブラウザ確認は Compose 起動後だけ（既定 CI では動かない）:

```powershell
cd apps/storefront
npx playwright install chromium
npx playwright test
```

Compose 起動後のヘルス:

```powershell
node scripts/compose-smoke.mjs http://localhost:8099/health http://localhost:8110/health
```

設計の詳細は [portfolio-plan](https://github.com/maeplego/portfolio-plan) の `portfolio-plan/commerce-platform/docs/` です。
