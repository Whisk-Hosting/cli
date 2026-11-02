import { serve } from "inngest/next";
import { functions } from "../../../functions";
import { inngest } from "../../../whisk";

// Function runs arrive here from the platform with the service identity (queue.endpoint in
// whisk.yaml).
export const dynamic = "force-dynamic";
export const { GET, POST, PUT } = serve({ client: inngest, functions });
