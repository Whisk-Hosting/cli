import { ModuleProvider, Modules } from "@medusajs/framework/utils"
import PaypalProviderService from "./service"

export default ModuleProvider(Modules.PAYMENT, { services: [PaypalProviderService] })
