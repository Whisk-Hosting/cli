import { ModuleProvider, Modules } from "@medusajs/framework/utils"
import WhiskFileService from "./service"

export default ModuleProvider(Modules.FILE, { services: [WhiskFileService] })
