import type { APIRoute } from "astro";
import { sql } from "../db.js";

export const GET: APIRoute = async () => {
  try {
    await sql`select 1`;
    return Response.json({ ok: true });
  } catch (err) {
    return Response.json({ ok: false, error: String(err) }, { status: 503 });
  }
};
