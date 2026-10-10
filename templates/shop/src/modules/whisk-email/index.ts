import { ModuleProvider, Modules } from "@medusajs/framework/utils"
import WhiskEmailService from "./service"

export default ModuleProvider(Modules.NOTIFICATION, { services: [WhiskEmailService] })
