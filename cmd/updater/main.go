package main

import (
	"log"

	"github.com/pdcgo/autoupdater"
)

var Variant string
var Storage string
var Version string
var AppEntryPoint string

func main() {
	log.Println("updater Version ", Version)
	log.Println("variant ", Variant)
	updater := autoupdater.Updater{
		Variant:       Variant,
		Storage:       Storage,
		AppEntryPoint: AppEntryPoint,
	}

	updater.Run()

}
