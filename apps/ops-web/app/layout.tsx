import "./globals.css";

export const dynamic = "force-dynamic";

export default function RootLayout({ children }: { children: React.ReactNode }) {
  return (
    <html lang="ja">
      <body>
        <div className="site-shell">
          <header className="site-header">
            <div className="site-brand">
              <strong>pf-commerce ops</strong>
              <span className="muted">在庫グリッド（学習用）</span>
            </div>
            <nav className="site-nav">
              <a href="/">在庫グリッド</a>
              <span className="pill">storefront は :3009</span>
            </nav>
          </header>
          <main className="site-main">{children}</main>
        </div>
      </body>
    </html>
  );
}
