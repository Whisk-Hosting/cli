// Makes a new shop ready to sell in New Zealand, and keeps it so on every deploy. It runs after
// the migrations (migrate.ts) and only adds what is missing, so staff changes in the admin are
// never undone. A new shop gets: New Zealand dollars with GST included in every price; a New
// Zealand region with 15% GST; a sales channel and the key the storefront uses; a stock
// location; standard delivery at NZ$10 that staff change in the admin; every payment
// provider whose keys are set (bank transfer always); and the table the website's forms are
// kept in.
import type { ExecArgs } from "@medusajs/framework/types"
import { ContainerRegistrationKeys, Modules } from "@medusajs/framework/utils"
import {
  createApiKeysWorkflow, createRegionsWorkflow, createSalesChannelsWorkflow, createShippingOptionsWorkflow,
  createShippingProfilesWorkflow, createStockLocationsWorkflow, createTaxRegionsWorkflow, linkSalesChannelsToApiKeyWorkflow,
  linkSalesChannelsToStockLocationWorkflow, updateRegionsWorkflow, updateStoresWorkflow,
} from "@medusajs/medusa/core-flows"
import { paymentProvidersFrom, providerIds, tradeOrdering } from "../lib/settings"
import { missingProviders, storeName } from "../lib/setup"

export default async function setup({ container }: ExecArgs) {
  const logger = container.resolve(ContainerRegistrationKeys.LOGGER)
  const link = container.resolve(ContainerRegistrationKeys.LINK)
  const query = container.resolve(ContainerRegistrationKeys.QUERY)
  const stores = container.resolve(Modules.STORE)
  const channels = container.resolve(Modules.SALES_CHANNEL)
  const regions = container.resolve(Modules.REGION)
  const taxes = container.resolve(Modules.TAX)
  const locations = container.resolve(Modules.STOCK_LOCATION)
  const fulfillment = container.resolve(Modules.FULFILLMENT)

  const [store] = await stores.listStores({}, { take: 1, relations: ["supported_currencies"] })

  // Sales channel.
  let [channel] = store.default_sales_channel_id
    ? await channels.listSalesChannels({ id: store.default_sales_channel_id })
    : await channels.listSalesChannels({}, { take: 1 })
  if (!channel) {
    const { result } = await createSalesChannelsWorkflow(container).run({ input: { salesChannelsData: [{ name: "Online shop" }] } })
    channel = result[0]
    logger.info("setup: made the Online shop sales channel")
  }

  // Store: its name, NZD with GST included, the sales channel.
  const currencies = store.supported_currencies ?? []
  const update: Record<string, unknown> = {}
  if (!currencies.some((c) => c.currency_code === "nzd")) {
    update.supported_currencies = [
      { currency_code: "nzd", is_default: true, is_tax_inclusive: true },
      ...currencies.map((c) => ({ currency_code: c.currency_code, is_default: false })),
    ]
  }
  const name = storeName(store.name, process.env.WHISK_APP_NAME)
  if (name !== store.name) update.name = name
  if (store.default_sales_channel_id !== channel.id) update.default_sales_channel_id = channel.id
  if (Object.keys(update).length) {
    await updateStoresWorkflow(container).run({ input: { selector: { id: store.id }, update } })
    logger.info(`setup: store ${Object.keys(update).join(", ")}`)
  }

  // Region and its payment providers.
  const wanted = providerIds(paymentProvidersFrom(process.env, tradeOrdering()))
  let [region] = await regions.listRegions({ currency_code: "nzd" }, { take: 1 })
  if (!region) {
    const { result } = await createRegionsWorkflow(container).run({
      input: { regions: [{ name: "New Zealand", currency_code: "nzd", countries: ["nz"], automatic_taxes: true, is_tax_inclusive: true, payment_providers: wanted }] },
    })
    region = result[0]
    logger.info("setup: made the New Zealand region")
  } else {
    const { data } = await query.graph({ entity: "region", fields: ["id", "payment_providers.id"], filters: { id: region.id } })
    const have = ((data[0] as any)?.payment_providers ?? []).map((p: { id: string }) => p.id)
    const add = missingProviders(have, wanted)
    if (add.length) {
      await updateRegionsWorkflow(container).run({ input: { selector: { id: region.id }, update: { payment_providers: [...have, ...add] } } })
      logger.info(`setup: region now takes ${add.join(", ")}`)
    }
  }

  // GST.
  const [taxRegion] = await taxes.listTaxRegions({ country_code: "nz" }, { take: 1 })
  if (!taxRegion) {
    await createTaxRegionsWorkflow(container).run({
      input: [{ country_code: "nz", provider_id: "tp_system", default_tax_rate: { rate: 15, code: "GST", name: "GST" } }],
    })
    logger.info("setup: GST at 15%")
  }

  // Stock location, linked to the sales channel and to manual fulfilment.
  let [location] = await locations.listStockLocations({}, { take: 1 })
  if (!location) {
    const { result } = await createStockLocationsWorkflow(container).run({
      input: { locations: [{ name: "Main", address: { address_1: "", city: "", country_code: "NZ" } }] },
    })
    location = result[0]
    await link.create({ [Modules.STOCK_LOCATION]: { stock_location_id: location.id }, [Modules.FULFILLMENT]: { fulfillment_provider_id: "manual_manual" } })
    await linkSalesChannelsToStockLocationWorkflow(container).run({ input: { id: location.id, add: [channel.id] } })
    await updateStoresWorkflow(container).run({ input: { selector: { id: store.id }, update: { default_location_id: location.id } } })
    logger.info("setup: made the Main stock location")
  }

  // Delivery: one shipping profile, one zone (New Zealand), standard delivery.
  let [profile] = await fulfillment.listShippingProfiles({ type: "default" }, { take: 1 })
  if (!profile) {
    const { result } = await createShippingProfilesWorkflow(container).run({ input: { data: [{ name: "Default", type: "default" }] } })
    profile = result[0]
  }
  const [set] = await fulfillment.listFulfillmentSets({}, { take: 1, relations: ["service_zones"] })
  if (!set) {
    const made = await fulfillment.createFulfillmentSets({
      name: "Delivery",
      type: "shipping",
      service_zones: [{ name: "New Zealand", geo_zones: [{ country_code: "nz", type: "country" }] }],
    })
    await link.create({ [Modules.STOCK_LOCATION]: { stock_location_id: location.id }, [Modules.FULFILLMENT]: { fulfillment_set_id: made.id } })
    await createShippingOptionsWorkflow(container).run({
      input: [
        {
          name: "Standard delivery",
          price_type: "flat",
          provider_id: "manual_manual",
          service_zone_id: made.service_zones[0].id,
          shipping_profile_id: profile.id,
          type: { label: "Standard", description: "Delivered in 1 to 5 working days.", code: "standard" },
          prices: [{ currency_code: "nzd", amount: 10 }, { region_id: region.id, amount: 10 }],
          rules: [
            { attribute: "enabled_in_store", value: "true", operator: "eq" },
            { attribute: "is_return", value: "false", operator: "eq" },
          ],
        },
      ],
    })
    logger.info("setup: standard delivery across New Zealand")
  }

  // The key the storefront sends with every store request.
  const { data: keys } = await query.graph({ entity: "api_key", fields: ["id"], filters: { type: "publishable", revoked_at: null } as any })
  if (!keys.length) {
    const { result } = await createApiKeysWorkflow(container).run({ input: { api_keys: [{ title: "Storefront", type: "publishable", created_by: "" }] } })
    await linkSalesChannelsToApiKeyWorkflow(container).run({ input: { id: result[0].id, add: [channel.id] } })
    logger.info("setup: made the storefront's key")
  }

  // The website's form entries (src/api/middlewares.ts, formPost).
  await container.resolve(ContainerRegistrationKeys.PG_CONNECTION).raw(`create table if not exists form_entries (
    id bigint generated always as identity primary key,
    form text not null,
    page text,
    data jsonb not null,
    created_at timestamptz not null default now()
  )`)
}
