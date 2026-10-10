# Errors: MANAGED_PRODUCTS_ORG_TAKEN

An operator marking a business as Whisk's own while another business is marked is refused with
`MANAGED_PRODUCTS_ORG_TAKEN` (409), whose `details.marked` names the marked business's slug. The
wire type `ProductsOrg` {id, slug, whisk_products} is what marking a business answers.
