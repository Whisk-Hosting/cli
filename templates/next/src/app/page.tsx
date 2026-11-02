import { headers } from "next/headers";
import { identity, log } from "../whisk";

const appName = process.env.WHISK_APP_NAME ?? "whisk-next";

export const metadata = { title: appName };

// Public (routes.public in whisk.yaml), and drawn for each request: it greets whoever is
// signed in.
export default async function Home() {
  const h = await headers();
  const id = identity((n) => h.get(n) ?? undefined);
  log.info({ request_id: id.requestId, method: "GET", path: "/", user: id.email }, "request");
  return (
    <main>
      <h1>{appName}</h1>
      <p>{id.audience === "anonymous" ? "You are not signed in." : `Signed in as ${id.name || id.email}.`}</p>
      <p>
        <a href="/notes">Notes</a> · <a href="/.whisk/login?return=/">Sign in</a> · <a href="/.whisk/logout?return=/">Sign out</a>
      </p>
    </main>
  );
}
