export default function RootLayout({ children }: { children: React.ReactNode }) {
  return (
    <html lang="en">
      <body style={{ fontFamily: "system-ui", maxWidth: "40rem", margin: "4rem auto", padding: "0 1rem" }}>{children}</body>
    </html>
  );
}
