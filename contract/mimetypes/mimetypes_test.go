package mimetypes

import "testing"

func TestByName(t *testing.T) {
	for name, want := range map[string]string{
		"Veetu-Seismic-Brace-Kit-3mm.step": "model/step",
		"part.STP":                         "model/step",
		"trapeze.glb":                      "model/gltf-binary",
		"scene.gltf":                       "model/gltf+json",
		"bracket.stl":                      "model/stl",
		"drawing.igs":                      "model/iges",
		"plan.dwg":                         "image/vnd.dwg",
		"print.3mf":                        "model/3mf",
		"brochure.pdf":                     "application/pdf",
		"sheet.xlsx":                       "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet",
		"notes.docx":                       "application/vnd.openxmlformats-officedocument.wordprocessingml.document",
		"photo.JPG":                        "image/jpeg",
		"clip.mp4":                         "video/mp4",
		"archive.zip":                      "application/zip",
		"data.csv":                         "text/csv",
		"dir/inside.txt":                   "text/plain",
		"README":                           "",
		"weird.nosuchextension":            "",
	} {
		if got := ByName(name); got != want {
			t.Errorf("ByName(%q) = %q, want %q", name, got, want)
		}
	}
}

func TestCompressible(t *testing.T) {
	for ct, want := range map[string]bool{
		"model/step":                true, // mime-db leaves it open; a CAD text format
		"model/gltf-binary":         true, // mime-db says so
		"text/plain; charset=utf-8": true,
		"application/json":          true,
		"image/svg+xml":             true,
		"application/pdf":           false,
		"image/jpeg":                false,
		"application/zip":           false,
		"video/mp4":                 false,
		"application/octet-stream":  false,
		"":                          false,
	} {
		if got := Compressible(ct); got != want {
			t.Errorf("Compressible(%q) = %v, want %v", ct, got, want)
		}
	}
}

func TestSettle(t *testing.T) {
	for _, c := range []struct{ name, given, want string }{
		{"brace.step", "text/plain", "model/step"},
		{"brace.step", "text/plain; charset=utf-8", "model/step"},
		{"brace.step", "", "model/step"},
		{"trapeze.glb", "application/octet-stream", "model/gltf-binary"},
		{"trapeze.glb", "model/gltf-binary", "model/gltf-binary"},
		{"notes.txt", "text/plain; charset=utf-8", "text/plain; charset=utf-8"},
		{"photo.jpg", "image/png", "image/png"}, // a specific type the client chose stands
		{"README", "text/plain", "text/plain"},
		{"README", "", "application/octet-stream"},
		{"blob.nosuchextension", "application/x-custom", "application/x-custom"},
	} {
		if got := Settle(c.name, c.given); got != c.want {
			t.Errorf("Settle(%q, %q) = %q, want %q", c.name, c.given, got, c.want)
		}
	}
}
