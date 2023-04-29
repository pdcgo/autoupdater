# autoupdater

untuk creating updater 

```
package main

import "github.com/pdcgo/autoupdater"

func main() {
	up := autoupdater.Publiser{
		Version:       "1.0.0",
		Storage:       "storage_google_cloud",
		Variant:       "beta",
		OutputDir:     "dist",
		AppEntryPoint: "bin/ss.exe", // entry point yang akan dipanggil updater
	}

	up.Run()
}
```