package autoupdater

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"cloud.google.com/go/storage"
	"github.com/schollz/progressbar/v3"
)

type ConfigAccount struct {
	Email string `yaml:"email"`
	Pwd   string `yaml:"pwd"`
}

type License struct {
	Lisensi *ConfigAccount `yaml:"lisensi"`
}

type AppArchiver struct {
	AppName string `json:"app_name"`
	Meta    *Meta  `json:"meta"`
	Bucket  *storage.BucketHandle
}

func (arcv *AppArchiver) GetMeta() *Meta {
	log.Println("Getting meta file..")
	ctx := context.Background()
	rcv, err := arcv.Bucket.Object(fmt.Sprintf("%s/meta.json", arcv.AppName)).NewReader(ctx)
	if err != nil {
		log.Println("Meta file does't exist")
		log.Println("Creating meta file")
		arcv.Meta = &Meta{
			CurrentVersion: "1.0.0",
			LastVersionUrl: "",
		}
		arcv.UploadMeta()
		return arcv.Meta
	}
	defer rcv.Close()
	var meta Meta

	metaData, _ := io.ReadAll(rcv)
	json.Unmarshal(metaData, &meta)
	arcv.Meta = &meta
	return arcv.Meta
}

func (arcv *AppArchiver) UploadMeta() error {
	ctx := context.Background()
	ctx, cancel := context.WithTimeout(ctx, time.Second*10)
	defer cancel()

	object := arcv.Bucket.Object(fmt.Sprintf("%s/meta.json", arcv.AppName))
	log.Println("Upload meta file")

	blob := object.NewWriter(ctx)
	blob.ContentType = "text/plain"

	data, _ := json.Marshal(arcv.Meta)
	dataLength := binary.Size(data)
	payload := bytes.NewBuffer(data)

	bar := progressbar.DefaultBytes(
		int64(dataLength),
		"uploading",
	)
	io.Copy(io.MultiWriter(blob, bar), payload)

	blob.Close()
	acl := object.ACL()
	if err := acl.Set(ctx, storage.AllUsers, storage.RoleReader); err != nil {
		return errors.New("error while make public file")
	}
	return nil
}

func (arcv *AppArchiver) UploadArchive(filename string, version string) error {
	log.Println("Upload app file")
	ctx := context.Background()
	ctx, cancel := context.WithTimeout(ctx, time.Second*600)
	defer cancel()

	object := arcv.Bucket.Object(fmt.Sprintf("%s/app_v%s.zip", arcv.AppName, version))
	blob := object.NewWriter(ctx)
	blob.ChunkSize = 262144

	file, _ := os.Open(filename)
	defer file.Close()
	fileInfo, _ := file.Stat()

	bar := progressbar.DefaultBytes(
		fileInfo.Size(),
		"uploading",
	)
	io.Copy(io.MultiWriter(blob, bar), file)

	blob.Close()
	acl := object.ACL()
	if err := acl.Set(ctx, storage.AllUsers, storage.RoleReader); err != nil {
		return errors.New("error while make public file")
	}
	attrs, _ := object.Attrs(ctx)
	meta := &Meta{
		CurrentVersion: version,
		LastVersionUrl: fmt.Sprintf("https://storage.googleapis.com/%s/%s", attrs.Bucket, attrs.Name),
	}
	arcv.Meta = meta
	arcv.UploadMeta()
	return nil
}

func (arcv *AppArchiver) GetListArchive() error {
	return errors.New("not implemented")
}

func ConfigureClient(appName string, bucketId string) (*AppArchiver, error) {
	log.Println("Set client")
	os.Setenv("GOOGLE_APPLICATION_CREDENTIALS", "credentials.json")
	ctx := context.Background()
	client, err := storage.NewClient(ctx)
	if err != nil {
		return nil, err
	}
	bkt := client.Bucket(bucketId)
	archiver := &AppArchiver{
		AppName: appName,
		Bucket:  bkt,
	}
	return archiver, nil
}

// func createConfig() {
// 	config, err := os.Create(outputDir + "/config.yml")
// 	if err != nil {
// 		panic(err)
// 	}
// 	defer config.Close()

// 	configAccount := &ConfigAccount{
// 		Email: "example@example.com",
// 		Pwd:   "pwd",
// 	}
// 	license := &License{
// 		Lisensi: configAccount,
// 	}
// 	yamlData, err := yaml.Marshal(license)
// 	if err != nil {
// 		fmt.Printf("Error while Marshaling. %v", err)
// 	}
// 	config.Write(yamlData)
// }

func appendToZipFile(src string, dest string, zipw *zip.Writer) error {
	destapp := dest
	if strings.HasPrefix(dest, "/") || strings.HasPrefix(dest, `\`) {
		destapp = "." + dest
	}
	log.Println("Copying ", src, " to ", destapp)
	file, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("failed to open %s: %s", src, err)
	}
	defer file.Close()

	wr, err := zipw.Create(destapp)
	if err != nil {
		msg := "failed to create entry for %s in zip file: %s"
		return fmt.Errorf(msg, dest, err)
	}
	fileStat, _ := file.Stat()
	bar := progressbar.Default(
		fileStat.Size(),
		"compressed",
	)

	if _, err := io.Copy(io.MultiWriter(bar, wr), file); err != nil {
		return fmt.Errorf("failed to write %s to zip: %s", src, err)
	}

	return nil
}

type BuildFunc func(outputdir string) (string, error)

type Publiser struct {
	Version       string
	Storage       string
	Variant       string
	AppEntryPoint string
	OutputDir     string
	BuildCmd      []BuildFunc
}

func (pub *Publiser) Run() {
	pub.createOutputDir()
	pub.buildUpdater()

	location, appendFile, close := pub.createdZippedFile()

	for _, handlerfn := range pub.BuildCmd {
		output, err := handlerfn(pub.OutputDir)
		if err != nil {
			log.Panicln(err)
		}
		err = appendFile(output)
		if err != nil {
			log.Panicln(err)
		}
	}

	close()
	pub.uploadZipFile(location)

}

func (pub *Publiser) buildUpdater() {
	log.Println("create updater...")
	var outb, errb bytes.Buffer

	// var Variant string
	// var Storage string
	// var Version string
	// var AppEntryPoint string
	// flags := `"`
	flags := "-X 'main.Variant=" + pub.Variant + "'"
	flags += " -X 'main.Storage=" + pub.Storage + "'"
	flags += " -X 'main.Version=" + pub.Version + "'"
	flags += " -X 'main.AppEntryPoint=" + pub.AppEntryPoint + "'"
	// flags += `"`
	// go build  -o ./ -ldflags "-X 'main.Variant=chat'" ./cmd/updater
	updatefname := filepath.Join(pub.OutputDir, "./updater_"+pub.Variant+".exe")

	cmdBuild := exec.Command("go", "build", "-o", updatefname, "-ldflags", flags, "github.com/pdcgo/autoupdater/cmd/updater")
	cmdBuild.Stdout = &outb
	cmdBuild.Stderr = &errb

	err := cmdBuild.Run()

	if err != nil {
		fmt.Println("out:", outb.String(), "err:", errb.String())
		log.Panicln(err)
	}
}

func (pub *Publiser) createOutputDir() {
	log.Println("Create output dist")
	os.RemoveAll(pub.OutputDir)
	err := os.MkdirAll(pub.OutputDir, 0755)
	if err != nil {
		panic("fail to create dist directory")
	}
	os.Mkdir(filepath.Join(pub.OutputDir, "./bin/"), 0755)
}

func (pub *Publiser) uploadZipFile(ziplocation string) {
	archiver, err := ConfigureClient(pub.Variant, pub.Storage)
	if err != nil {
		panic(err)
	}
	archiver.GetMeta()
	archiver.UploadArchive(ziplocation, pub.Version)
	log.Println("Success upload archiver")
}

func (pub *Publiser) createdZippedFile() (string, func(buildpath string) error, func()) {
	location := filepath.Join(pub.OutputDir, pub.Variant+".zip")
	log.Println("creating", location)

	zipFile, err := os.Create(location)
	if err != nil {
		panic("failed create compressed file")
	}

	archive := zip.NewWriter(zipFile)

	return location, func(buildpath string) error {
			destination := strings.Split(buildpath, pub.OutputDir)
			dest := destination[len(destination)-1]
			return appendToZipFile(buildpath, dest, archive)
		}, func() {
			zipFile.Close()
		}
}

// func buildApp() {
// 	log.Println("Start building app")
// 	cmdBuild := exec.Command("wails", "build", "-o", "tiktok_chatbot.exe")
// 	cmdBuild.Stdin = os.Stdin
// 	cmdBuild.Stdout = os.Stdout
// 	cmdBuild.Stderr = os.Stderr
// 	cmdBuild.Run()
// }
