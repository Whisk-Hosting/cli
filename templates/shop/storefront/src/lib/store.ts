// The storefront's reads and writes against the store API, each a small function over api().
import type { AstroCookies } from "astro"
import { api, ApiError, query, type ShopContext } from "./medusa"

export const CART_COOKIE = "shop_cart"

type Api = ReturnType<typeof api>

const PRODUCT_FIELDS = "*variants.calculated_price,+variants.inventory_quantity,*variants.options,*options,*options.values,*images,*categories"
const CARD_FIELDS = "id,handle,title,thumbnail,*variants.calculated_price"
const CART_FIELDS = "*items,*items.variant,*items.variant.product,*shipping_methods,*shipping_address,*billing_address,*payment_collection,*payment_collection.payment_sessions,*promotions,+item_subtotal,+shipping_subtotal,+items.subtotal"

export const products = async (a: Api, shop: ShopContext, opts: { q?: string; category?: string; collection?: string; limit?: number; offset?: number; ids?: string[] }) =>
  a.get<{ products: any[]; count: number }>(
    `/store/products${query({
      region_id: shop.regionId,
      fields: CARD_FIELDS,
      q: opts.q,
      category_id: opts.category,
      collection_id: opts.collection,
      id: opts.ids,
      limit: opts.limit ?? 24,
      offset: opts.offset ?? 0,
      order: "-created_at",
    })}`,
  )

export const product = async (a: Api, shop: ShopContext, handle: string) => {
  const { products } = await a.get<{ products: any[] }>(`/store/products${query({ handle, region_id: shop.regionId, fields: PRODUCT_FIELDS, limit: 1 })}`)
  return products[0] ?? null
}

export const categories = async (a: Api) =>
  (await a.get<{ product_categories: any[] }>(`/store/product-categories${query({ fields: "id,name,handle,parent_category_id,rank", limit: 200 })}`)).product_categories

export const categoryByPath = async (a: Api, path: string) => {
  const { product_categories } = await a.get<{ product_categories: any[] }>(
    `/store/product-categories${query({ handle: path, fields: "id,name,handle,description,*category_children", limit: 1 })}`,
  )
  return product_categories[0] ?? null
}

export const collectionByHandle = async (a: Api, handle: string) => {
  const { collections } = await a.get<{ collections: any[] }>(`/store/collections${query({ handle, limit: 1 })}`)
  return collections[0] ?? null
}

export const cartIdOf = (cookies: AstroCookies) => cookies.get(CART_COOKIE)?.value ?? null

export const cart = async (a: Api, cookies: AstroCookies) => {
  const id = cartIdOf(cookies)
  if (!id) return null
  try {
    const { cart } = await a.get<{ cart: any }>(`/store/carts/${encodeURIComponent(id)}${query({ fields: CART_FIELDS })}`)
    return cart.completed_at ? null : cart
  } catch (e) {
    if (e instanceof ApiError && (e.status === 404 || e.status === 400)) return null
    throw e
  }
}

const keepCart = (cookies: AstroCookies, id: string) =>
  cookies.set(CART_COOKIE, id, { path: "/", httpOnly: true, sameSite: "lax", secure: true, maxAge: 60 * 60 * 24 * 30 })

export const dropCart = (cookies: AstroCookies) => cookies.delete(CART_COOKIE, { path: "/" })

// The visitor's cart, made the first time they add something.
export const ensureCart = async (a: Api, shop: ShopContext, cookies: AstroCookies) => {
  const existing = await cart(a, cookies)
  if (existing) return existing
  const { cart: made } = await a.post<{ cart: any }>("/store/carts", { region_id: shop.regionId })
  keepCart(cookies, made.id)
  return made
}

export const addLine = (a: Api, cartId: string, variantId: string, quantity: number) =>
  a.post<{ cart: any }>(`/store/carts/${encodeURIComponent(cartId)}/line-items`, { variant_id: variantId, quantity })

export const updateLine = (a: Api, cartId: string, lineId: string, quantity: number) =>
  quantity > 0
    ? a.post<{ cart: any }>(`/store/carts/${encodeURIComponent(cartId)}/line-items/${encodeURIComponent(lineId)}`, { quantity })
    : a.del<{ parent: any }>(`/store/carts/${encodeURIComponent(cartId)}/line-items/${encodeURIComponent(lineId)}`)

export const me = async (a: Api) => {
  try {
    return (await a.get<{ customer: any }>(`/store/customers/me${query({ fields: "*addresses" })}`)).customer
  } catch (e) {
    if (e instanceof ApiError && (e.status === 401 || e.status === 403)) return null
    throw e
  }
}

export const myOrders = async (a: Api, limit = 20, offset = 0) =>
  a.get<{ orders: any[]; count: number }>(`/store/orders${query({ limit, offset, order: "-created_at", fields: "id,display_id,created_at,total,currency_code,status,fulfillment_status,payment_status,*items" })}`)

export const order = async (a: Api, id: string) =>
  (await a.get<{ order: any }>(`/store/orders/${encodeURIComponent(id)}${query({ fields: "id,display_id,email,customer_id,created_at,status,payment_status,fulfillment_status,currency_code,*items,*shipping_address,*shipping_methods,+item_total,+item_subtotal,+shipping_total,+shipping_subtotal,+items.subtotal,+tax_total,+discount_total,+total,*payment_collections.payments,*payment_collections.payment_sessions" })}`)).order

// After a customer signs in, the cart they were filling becomes theirs.
export const claimCart = async (a: Api, cookies: AstroCookies) => {
  const id = cartIdOf(cookies)
  if (!id) return
  try {
    await a.post(`/store/carts/${encodeURIComponent(id)}/customer`, {})
  } catch {
    // A cart that cannot be claimed (already completed, or someone else's) is left as it is.
  }
}
