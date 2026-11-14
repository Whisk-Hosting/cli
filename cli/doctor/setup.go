package doctor

// A shop works for its customers only once the business has given what it needs: where orders
// and enquiries go, how it is paid, how it delivers. The shop keeps that list at a route the
// manifest names in admin.setup, so whisk deploy prints what is missing and the Whisk bar says
// "Finish setup" (SKILL.md, admin). W176 asks a shop for the list.

// shopSetupPath is the list the shop template and Whisk's commerce engine answer.
const shopSetupPath = "/admin/whisk/setup"

func w176(r Repo, _ Context) outcome {
	m := r.Manifest
	if m.Commerce != "medusa" || (m.Admin != nil && m.Admin.Setup != "") {
		return outcome{}
	}
	return one("W176", manifestFile, yamlLine(r.ManifestNode, "/commerce"),
		"The shop names no setup list, so whisk deploy cannot say what the business still has to give before it takes orders; add admin.setup: "+shopSetupPath+" to whisk.yaml.")
}
