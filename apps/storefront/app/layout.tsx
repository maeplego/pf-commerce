import "./globals.css";

export const dynamic = "force-dynamic";

export default function RootLayout({ children }: { children: React.ReactNode }) {
  return (
    <html lang="ja">
      <body>
        <div className="site-shell">
          <header className="site-header">
            <div className="site-brand">
              <a href="/" className="brand-link">
                <strong>pf-commerce</strong>
              </a>
              <span className="muted">学習用モジュラモノリス</span>
            </div>
            <nav className="site-nav">
              <a href="/">カタログ</a>
              <a href="/demo">在庫1の同時購入デモ</a>
              <span className="pill">ops-web は :3010</span>
            </nav>
          </header>
          <main className="site-main">{children}</main>
        </div>
      </body>
    </html>
  );
}
