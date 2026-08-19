export const dynamic = "force-dynamic";

export default function RootLayout({ children }: { children: React.ReactNode }) {
  return (
    <html lang="ja">
      <body style={{ fontFamily: "system-ui, sans-serif", margin: "1.5rem", maxWidth: 960, lineHeight: 1.5 }}>
        <header style={{ marginBottom: "1.5rem", borderBottom: "1px solid #ddd", paddingBottom: "0.75rem" }}>
          <a href="/" style={{ textDecoration: "none", color: "inherit" }}>
            <strong>pf-commerce</strong>
          </a>
          <span style={{ color: "#666", marginLeft: "0.75rem", fontSize: "0.9rem" }}>学習用モジュラモノリス</span>
          <nav style={{ marginTop: "0.5rem" }}>
            <a href="/">カタログ</a>
            {" · "}
            <a href="/demo">在庫1の同時購入デモ</a>
          </nav>
        </header>
        {children}
      </body>
    </html>
  );
}
