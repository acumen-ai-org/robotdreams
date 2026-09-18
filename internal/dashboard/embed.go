package dashboard

import (
	"embed"
	"io/fs"
)

//go:generate make -C ../.. ui-build

//go:embed all:web
var webFS embed.FS

func Built() bool {
	_, err := fs.Stat(webFS, "web/dist/index.html")
	return err == nil
}

func siteRoot() string {
	if Built() {
		return "web/dist"
	}
	return "web/placeholder"
}
