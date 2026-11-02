import { serve } from "inngest/astro";
import { functions } from "../../functions.js";
import { inngest } from "../../whisk.js";

// Function runs arrive here from the platform with the service identity (queue.endpoint in
// whisk.yaml).
export const { GET, POST, PUT } = serve({ client: inngest, functions });
