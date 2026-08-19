export const dynamic = "force-dynamic";

export default function RootLayout({ children }: { children: React.ReactNode }) {
  return (
    <html lang="ja">
      <body style={{ fontFamily: "system-ui, sans-serif", margin: "1.5rem", maxWidth: 1100, lineHeight: 1.5 }}>
        <header style={{ marginBottom: "1.5rem", borderBottom: "1px solid #ddd", paddingBottom: "0.75rem" }}>
          <strong>pf-commerce ops</strong>
          <span style={{ color: "#666", marginLeft: "0.75rem", fontSize: "0.9rem" }}>在庫グリッド（学習用）</span>
        </header>
        {children}
      </body>
    </html>
  );
}
