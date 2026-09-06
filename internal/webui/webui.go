package webui

import (
	"embed"
	"errors"
	"io/fs"
	"net/http"
)

//go:embed all:files
var files embed.FS

func New() (http.Handler, error) {
	assets, err := fs.Sub(files, "files/dist")
	if err != nil {
		return nil, errors.New("web assets are not built")
	}
	if _, err := fs.Stat(assets, "index.html"); err != nil {
		return nil, errors.New("web assets are not built")
	}
	return http.FileServer(http.FS(assets)), nil
}
