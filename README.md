# pf-commerce

P06 commerce-platform の製品リポジトリです。**学習用であり、本番 EC / 決済基盤の置き換えではありません。** 本物のカード番号は扱いません。

いまは **モジュラモノリス** です。カタログ・在庫・注文はプロセス内のモジュールで、DB は 1 つ（テーブル接頭辞で境界）。最初から 8 サービスに切ると、注文フローの正しさより配線が先に壊れます。分割は Compose で購入〜在庫不足が安定してからにします。

## 構成

| パス | 役割 |
| --- | --- |
| `apps/api` | Go API。catalog / inventory / cart / order（決済モックは order 内） |
| `apps/storefront` | Next.js。カタログと在庫 1 の同時購入デモ |
| `deploy/` | 単体 Compose（Postgres + API + storefront） |

K8s / overlay D、RabbitMQ、GraphQL BFF、注文のイベントストア化は **未着手**。商品画像は URL 文字列（P03 未接続）。認証は `X-Dev-User-Sub`（P01 OIDC は未配線）。

## 単体デモ

```powershell
cd deploy
copy .env.example .env
docker compose up -d --build
```

| URL | 用途 |
| --- | --- |
| http://localhost:3008 | ストアフロント |
| http://localhost:3008/demo | 在庫 1 を buyer-a / buyer-b が同時購入 |
| http://localhost:8098/health | API liveness |
| http://localhost:8098/ready | API readiness（Postgres ping） |

シード: `MUG-1` 在庫 1、`TEE-1` 在庫 20、`STK-1` 在庫 0。金額は整数円。

### 在庫不足デモ

1. `/demo` を開く
2. 左右の「MUG-1 を 1点買う」をほぼ同時に押す
3. 片方 `paid`、もう片方 `inventory_shortage`（引当を戻す）

API だけなら:

```powershell
curl http://localhost:8098/v1/products
curl -H "X-Dev-User-Sub: buyer-a" -H "Content-Type: application/json" `
  -d '{"idempotencyKey":"key-a","lines":[{"productId":"<MUG-ID>","qty":1}]}' `
  http://localhost:8098/v1/checkout
```

## テスト

```powershell
cd apps/api
go test ./...
```

メモリ実装。Postgres が必要な integration テストはこのスライスには無い（不足の契約は `TryReserve` の原子更新と同じ）。

## 既知の制限

- サービス抽出前。共有ライブラリはまだ薄い（このリポジトリ内のモジュール）
- チェックアウトの引当行 insert は残高 UPDATE の次ステートメント（抽出時に 1 TX へ）
- 予約 TTL 切れは次の Reserve 時に回収。専用ワーカーなし
- 決済は常に成功するモック（テストだけ失敗を注入）
- ops-web / GraphQL / 出荷 / メール / 推薦スロットなし

設計: `project/portfolio-plan/commerce-platform/DESIGN.md`  
人間向け書類: `project/portfolio-plan/commerce-platform/docs/`
