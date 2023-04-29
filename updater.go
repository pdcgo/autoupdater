package autoupdater

import (
	"archive/zip"
	"encoding/gob"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/hashicorp/go-version"
	"github.com/schollz/progressbar/v3"
)

type Meta struct {
	CurrentVersion string `json:"current_version"`
	LastVersionUrl string `json:"last_version_url"`
}

func (meta *Meta) DefaultMeta() *Meta {
	meta.CurrentVersion = "1.0.0"
	meta.LastVersionUrl = ""
	return meta
}

type Updater struct {
	Variant       string `json:"app_name"`
	Storage       string `json:"storage"`
	AppEntryPoint string
}

func (updt *Updater) getUrlHost() string {
	return fmt.Sprintf("https://storage.googleapis.com/%s/%s/meta.json", updt.Storage, updt.Variant)
}

func (updt *Updater) GetLocalMeta() (*Meta, error) {
	fname := ".meta"

	_, err := os.Stat(fname)
	if os.IsNotExist(err) {
		return nil, errors.New("local meta is not exist")
	}

	for meta := range OpenMetaFile(fname) {
		return meta, nil
	}
	return nil, errors.New("local meta can't be loaded")
}

func (updt *Updater) GetRemoteMeta() *Meta {
	client := http.Client{}
	var meta Meta

	url := updt.getUrlHost()
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		panic("Can't Create Request.")
	}

	resp, err := client.Do(req)
	if err != nil {
		panic(err)
	}
	if resp.StatusCode == 404 {
		panic(err)
	}
	if resp.StatusCode != 200 {
		panic(err)
	}

	data, _ := io.ReadAll(resp.Body)
	json.Unmarshal(data, &meta)
	return &meta
}

func (updt *Updater) CheckUpdate() error {
	log.Println("Checking Update....")
	fname := ".meta"

	remoteMeta := updt.GetRemoteMeta()

	localMeta, err := updt.GetLocalMeta()
	if err != nil {
		localMeta = updt.GetRemoteMeta()

		metaFile, _ := os.Create(fname)
		enc := gob.NewEncoder(metaFile)
		enc.Encode(localMeta)

		cmdHide := exec.Command("attrib", "+h", fname)
		defer cmdHide.Run()

		log.Printf("new version available.. %s", localMeta.CurrentVersion)
		log.Println("update now..")
		updt.RunUpdate(remoteMeta)
		return nil
	}
	latestVersion, _ := version.NewVersion(remoteMeta.CurrentVersion)
	currentVersion, _ := version.NewVersion(localMeta.CurrentVersion)
	if latestVersion.GreaterThan(currentVersion) {
		log.Printf("new version available.. %s\n", latestVersion)
		log.Println("update now..")

		updt.RunUpdate(remoteMeta)
	}

	CreateMetaFile(remoteMeta)
	return nil
}

func (updt *Updater) RunUpdate(meta *Meta) {
	client := http.Client{}

	req, err := http.NewRequest("GET", meta.LastVersionUrl, nil)
	if err != nil {
		panic("Can't Create Request.")
	}

	resp, err := client.Do(req)
	if err != nil {
		panic(err)
	}
	contentLength := resp.ContentLength
	splitUrl := strings.Split(meta.LastVersionUrl, "/")
	fname := splitUrl[len(splitUrl)-1]
	file, _ := os.Create(fname)
	defer os.Remove(fname)
	defer file.Close()

	log.Printf("download %s", fname)
	bar := progressbar.DefaultBytes(
		contentLength,
		"downloading",
	)
	io.Copy(io.MultiWriter(file, bar), resp.Body)
	err = updt.ExtractZipFile(fname, "")
	if err != nil {
		panic(err)
	}
}

func (updt *Updater) ExtractZipFile(src string, dest string) error {
	archive, err := zip.OpenReader(src)
	if err != nil {
		return err
	}
	defer archive.Close()
	log.Printf("extract zip file %s", src)

	for _, f := range archive.File {
		filePath := filepath.Join(dest, f.Name)

		if f.FileInfo().IsDir() {
			os.MkdirAll(filePath, os.ModePerm)
			continue
		}

		if err := os.MkdirAll(filepath.Dir(filePath), os.ModePerm); err != nil {
			return err
		}

		destFile, err := os.OpenFile(filePath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, f.Mode())
		if err != nil {
			return err
		}

		fileInArchive, err := f.Open()
		if err != nil {
			return err
		}

		if _, err := io.Copy(destFile, fileInArchive); err != nil {
			return err
		}

		destFile.Close()
		fileInArchive.Close()
	}
	return nil
}

func (updt *Updater) DetachProcess(command string) {
	cmd := exec.Command("cmd", "/C", "start", command)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Run()
}

func OpenMetaFile(fileName string) chan *Meta {
	var metaData = make(chan *Meta)

	cmdUnhide := exec.Command("attrib", "-h", fileName)
	cmdHide := exec.Command("attrib", "+h", fileName)
	cmdUnhide.Run()
	defer cmdHide.Run()

	go func() {
		var meta Meta
		file, err := os.Open(fileName)
		if err != nil {
			panic(err)
		}
		defer file.Close()
		dec := gob.NewDecoder(file)
		dec.Decode(&meta)

		metaData <- &meta
	}()
	return metaData

}

func CreateMetaFile(meta *Meta) {
	fname := ".meta"
	cmdUnHide := exec.Command("attrib", "-h", fname)
	cmdUnHide.Run()
	metaFile, _ := os.Create(fname)
	enc := gob.NewEncoder(metaFile)
	enc.Encode(meta)
	cmdHide := exec.Command("attrib", "+h", fname)
	defer cmdHide.Run()
}

func (up *Updater) Run() {
	up.CheckUpdate()
	up.DetachProcess(up.AppEntryPoint)
}
