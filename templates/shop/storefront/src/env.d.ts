/// <reference types="astro/client" />

declare namespace App {
  interface Locals {
    // Handed in by the server (src/api/middlewares.ts) on every request.
    shop: import("./lib/medusa").ShopContext
  }
}
