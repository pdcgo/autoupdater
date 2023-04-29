package main

import "github.com/pdcgo/autoupdater"

func main() {
	up := autoupdater.Publiser{
		Version:       "1.0.0",
		Storage:       "asdasd",
		Variant:       "beta",
		OutputDir:     "dist",
		AppEntryPoint: "bin/ss.exe",
	}

	up.Run()
}
