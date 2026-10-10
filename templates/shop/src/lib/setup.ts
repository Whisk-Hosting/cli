// Pure decisions for src/scripts/setup.ts.

// The store's name: Medusa names a new store "Medusa Store"; a shop starts with its app's name
// in words ("acme-tools" is "Acme Tools"). A name staff have set is kept.
export const storeName = (current: string | null | undefined, appName: string | undefined): string => {
  if (current && current !== "Medusa Store") return current
  if (!appName) return current || "Shop"
  return appName.split("-").filter(Boolean).map((w) => w[0].toUpperCase() + w.slice(1)).join(" ")
}

// Providers to add to the region: those configured that it does not take yet. One staff have
// added is kept, and one whose keys were removed stays listed (Medusa skips a provider it has
// not loaded) so putting the keys back needs nothing else.
export const missingProviders = (have: string[], wanted: string[]): string[] => wanted.filter((p) => !have.includes(p))
