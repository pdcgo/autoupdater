package main

import (
	"bytes"
	"log"
	"os/exec"
	"path/filepath"

	"github.com/pdcgo/autoupdater"
)

func main() {
	up := autoupdater.Publiser{
		Version:       "2.0.0",
		Storage:       "tiktok_chat_artifact",
		Variant:       "beta",
		OutputDir:     "dist",
		AppEntryPoint: "./bin/sampleapp.exe",
		BuildCmd: []autoupdater.BuildFunc{func(outputdir string) (string, error) {
			log.Println("create aplication...")
			var outb, errb bytes.Buffer

			updatefname := filepath.Join(outputdir, "./bin/sampleapp.exe")

			cmdBuild := exec.Command("go", "build", "-o", updatefname, "github.com/pdcgo/autoupdater/cmd/sampleapp")
			cmdBuild.Stdout = &outb
			cmdBuild.Stderr = &errb

			err := cmdBuild.Run()

			if err != nil {
				return "", err
			}
			return updatefname, nil
		}},
	}

	up.Run()
}
