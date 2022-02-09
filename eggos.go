package main

import (
	"embed"

	_ "github.com/icexin/eggos"
	eggfs "github.com/icexin/eggos/fs"
	"github.com/icexin/eggos/fs/stripprefix"
	"github.com/spf13/afero"
)

//go:embed shorterdata
var shorterdataFS embed.FS

func MountShorterData() {
	roShorterdataFS := afero.FromIOFS{FS: shorterdataFS}
	cowShorterdataFS := afero.NewCopyOnWriteFs(roShorterdataFS, afero.NewMemMapFs())
	const target = "/7i"
	if err := eggfs.Mount(target, stripprefix.New("/", cowShorterdataFS)); err != nil {
		panic(err)
	}
}
