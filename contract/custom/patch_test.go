package custom

import (
	"reflect"
	"testing"
)

// The lines a diff adds to each file, by their numbers in the file as it is now.
func TestPatchAdded(t *testing.T) {
	patch := "diff --git a/src/deals/won.ts b/src/deals/won.ts\nindex 1..2 100644\n--- a/src/deals/won.ts\n+++ b/src/deals/won.ts\n" +
		"@@ -4,0 +5 @@ export const won\n+tell();\n@@ -9,2 +10,3 @@\n-old();\n-older();\n+one();\n+++two;\n+three();\n" +
		"diff --git a/custom/changes/tell.yaml b/custom/changes/tell.yaml\nnew file mode 100644\n--- /dev/null\n+++ b/custom/changes/tell.yaml\n" +
		"@@ -0,0 +1,2 @@\n+asked: Tell the warehouse.\n+touches: [src/deals/won.ts]\n\\ No newline at end of file\n" +
		"diff --git a/old.ts b/old.ts\ndeleted file mode 100644\n--- a/old.ts\n+++ /dev/null\n@@ -1 +0,0 @@\n-gone();\n" +
		"diff --git \"a/caf\\303\\251.ts\" \"b/caf\\303\\251.ts\"\n--- \"a/caf\\303\\251.ts\"\n+++ \"b/caf\\303\\251.ts\"\n@@ -1,2 +1,2 @@\n kept();\n-was();\n+is();\n" +
		"diff --git a/logo.png b/logo.png\nBinary files a/logo.png and b/logo.png differ\n"
	want := map[string][]Line{
		"src/deals/won.ts":         {{Number: 5, Text: "tell();"}, {Number: 10, Text: "one();"}, {Number: 11, Text: "++two;"}, {Number: 12, Text: "three();"}},
		"custom/changes/tell.yaml": {{Number: 1, Text: "asked: Tell the warehouse."}, {Number: 2, Text: "touches: [src/deals/won.ts]"}},
		"café.ts":                  {{Number: 2, Text: "is();"}},
	}
	if got := PatchAdded(patch); !reflect.DeepEqual(got, want) {
		t.Errorf("PatchAdded =\n%+v\nwant\n%+v", got, want)
	}
	if hunkStart("@@ -1 +0,0 @@") != 1 || hunkStart("@@ bad") != 0 || len(PatchAdded("")) != 0 {
		t.Error("hunkStart or an empty diff")
	}
}
