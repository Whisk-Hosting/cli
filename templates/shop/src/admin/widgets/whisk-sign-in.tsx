// The admin's sign-in page sends the business's team to sign in through Whisk instead
// (src/api/auth/whisk). Nobody has an admin password.
import { defineWidgetConfig } from "@medusajs/admin-sdk"
import { useEffect } from "react"

const WhiskSignIn = () => {
  useEffect(() => {
    const from = new URLSearchParams(window.location.search).get("from") ?? ""
    const back = from.startsWith("/") ? `/app${from}` : "/app"
    window.location.replace(`/auth/whisk?return=${encodeURIComponent(back)}`)
  }, [])
  return null
}

export const config = defineWidgetConfig({ zone: "login.before" })

export default WhiskSignIn
