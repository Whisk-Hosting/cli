import { ModuleProvider, Modules } from "@medusajs/framework/utils"
import WindcaveProviderService from "./service"

export default ModuleProvider(Modules.PAYMENT, { services: [WindcaveProviderService] })
