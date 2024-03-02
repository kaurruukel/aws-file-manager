package main

import (
	"bytes"
	"context"
	"fmt"
	"html/template"
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/feature/s3/manager"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
)

type Folder struct {
	Name     string
	Files    []File
	Folders  []Folder
	Path     string
	Selector string
}

type File struct {
	Name     string
	Size     int64
	Path     string
	Selector string
}

func main() {
	app, err := newApp()
	if err != nil {
		log.Fatal(err)
	}
	port := 8000
	fmt.Printf("Listening on port %v", port)
	mux := http.NewServeMux()
	mux.HandleFunc("GET /home", app.handleGetRoot)
	mux.HandleFunc("GET /explorer", app.returnExplorerTemplate)
	mux.HandleFunc("GET /show-overlay", app.showOverlay)
	mux.HandleFunc("GET /create-folder", app.createFolder)
	mux.HandleFunc("POST /upload-file", app.postFile)
	mux.HandleFunc("GET /download-file", app.downloadHandler)
	mux.HandleFunc("DELETE /delete-obj", app.deleteObj)
	fs := http.Dir("static")
	mux.Handle("/static/", http.StripPrefix("/static/", http.FileServer(fs)))
	// mux.HandleFunc("/", unhandledRequest)
	http.ListenAndServe(":"+strconv.Itoa(port), mux)
}

type App struct {
	s3Client *s3.Client
	bucket   string
}

func newApp() (App, error) {
	cfg, err := config.LoadDefaultConfig(context.TODO())
	if err != nil {
		return App{}, err
	}
	client := s3.NewFromConfig(cfg)
	return App{s3Client: client}, nil
}

func (a *App) handleGetRoot(w http.ResponseWriter, r *http.Request) {
	b, err := a.getAllBuckets()

	// template, err := template.New("index.html").ParseFiles("templates/index.html")
	tmpl, err := template.ParseFiles("templates/index.html")
	if err != nil {
		log.Panicf("Error creating html template: %v", err)
		return
	}
	err = tmpl.Execute(w, b)
	if err != nil {
		log.Panicf("Error executing html template: %v", err)
		return
	}
}

func (a *App) postFile(w http.ResponseWriter, r *http.Request) {
	file, handler, err := r.FormFile("aws")
	k := r.URL.Query().Get("path")

	if err != nil {
		log.Panicf("Error reading file from body: %v", err)
	}
	defer file.Close()

	uploader := manager.NewUploader(a.s3Client)
	_, err = uploader.Upload(context.TODO(), &s3.PutObjectInput{
		Bucket: aws.String(a.bucket),
		Key:    aws.String(k + handler.Filename),
		Body:   file,
	})
	if err != nil {
		log.Panicf("Error uploading file:  %v", err)
	}

	tmpl, err := template.ParseFiles("templates/folder.html")
	if err != nil {
		log.Panicf("Error creating html template: %v", err)
		return
	}
	f := `
		<div class="file border border-black rounded flex p-2" id="{{ .Selector }}">
			<p class="my-auto w-full h-full">
				{{ .Name }}
			</p>
			{{ template "Delete" . }}
			{{ template "Download" . }}
		</div>
	`
	tmpl, err = tmpl.Parse(f)
	if err != nil {
		log.Panicf("Error creating html template: %v", err)
		return
	}

	fd := File{
		Name:     handler.Filename,
		Selector: makeStringSelectorSafe(k),
		Path:     k,
	}
	err = tmpl.Execute(w, fd)
	if err != nil {
		log.Panicf("Error executing html template: %v", err)
		return
	}
}

func (a *App) downloadHandler(w http.ResponseWriter, r *http.Request) {
	k := r.URL.Query().Get("key")
	result, err := a.s3Client.GetObject(context.TODO(), &s3.GetObjectInput{
		Bucket: aws.String("testing-for-golang"),
		Key:    aws.String(k),
	})
	if err != nil {
		log.Panicf("Couldn't GetObject: %s", err)
	}
	defer result.Body.Close()
	s := strings.Split(k, "/")

	w.Header().Set("Content-Disposition", "attachment; filename="+s[len(s)-1])
	w.Header().Set("Content-Type", "application/octet-stream")

	if _, err := io.Copy(w, result.Body); err != nil {

		http.Error(w, err.Error(), http.StatusInternalServerError)
	}

	return
}

func (a *App) getAllBuckets() ([]string, error) {
	r, err := a.s3Client.ListBuckets(context.TODO(), nil)
	var buckets []string
	for _, b := range r.Buckets {
		buckets = append(buckets, *b.Name)
	}
	return buckets, err

}

func (a *App) listfilesInBucket(w http.ResponseWriter, b string) (*Folder, error) {
	output, err := a.s3Client.ListObjectsV2(context.TODO(),
		&s3.ListObjectsV2Input{
			Bucket: aws.String(b),
		},
	)
	if err != nil {
		log.Panicf("problem: %v", err)
	}
	f := Folder{
		Selector: "root",
	}
	for _, o := range output.Contents {
		sliced := strings.Split(*o.Key, "/")

		if sliced[len(sliced)-1] == "" {
			sliced = sliced[:len(sliced)-1]
		}
		path := sliced
		name := sliced[len(sliced)-1]

		if string(string(*o.Key)[len(*o.Key)-1]) == "/" {
			newFolder := Folder{
				Name:     name,
				Path:     *o.Key,
				Selector: makeStringSelectorSafe(*o.Key),
			}
			f.Folders, f.Files = addObject(path, newFolder, f.Folders, f.Files)
		} else {
			newFile := File{
				Name:     name,
				Size:     *o.Size,
				Selector: makeStringSelectorSafe(*o.Key),
				Path:     *o.Key,
			}
			f.Folders, f.Files = addObject(path, newFile, f.Folders, f.Files)
		}
	}
	return &f, nil
}

func addObject(path []string, toAdd interface{}, folders []Folder, files []File) ([]Folder, []File) {
	if len(folders) == 0 {
		switch v := toAdd.(type) {
		case Folder:
			folders = []Folder{v}
			return folders, files
		case File:
			files = append(files, v)
			return folders, files
		}
	}

	for i, folder := range folders {
		if folder.Name == path[0] && len(path) == 2 {
			switch v := toAdd.(type) {
			case Folder:
				folder.Folders = append(folder.Folders, v)
				folders[i] = folder
				return folders, files
			case File:
				folder.Files = append(folder.Files, v)
				folders[i] = folder
				return folders, files
			}
		}
		if len(folder.Folders) > 0 {
			folder.Folders, folder.Files = addObject(path[1:], toAdd, folder.Folders, folder.Files)
			folders[i] = folder
		}
	}
	return folders, files
}

func (a *App) deleteObj(w http.ResponseWriter, r *http.Request) {
	p := r.URL.Query().Get("path")

	d := []types.ObjectIdentifier{{Key: aws.String(p)}}
	// if the path ends with a "/" then it is a folder and we first need to delete its childern
	if string(p[len(p)-1]) == "/" {
		output, err := a.s3Client.ListObjectsV2(context.TODO(),
			&s3.ListObjectsV2Input{
				Bucket: aws.String(a.bucket),
			},
		)
		if err != nil {
			log.Panicf("error retrieving files: %v", err)
		}

		for _, o := range output.Contents {
			if strings.HasPrefix(*o.Key, p) && p != *o.Key {
				d = append([]types.ObjectIdentifier{{Key: aws.String(*o.Key)}}, d...)
			}

		}
	}

	_, err := a.s3Client.DeleteObjects(context.TODO(), &s3.DeleteObjectsInput{
		Bucket: aws.String(a.bucket),
		Delete: &types.Delete{Objects: d},
	})
	if err != nil {
		log.Panicf("error deleting obj %v: %v", p, err)
	}

	w.Header().Set("HX-Trigger", "delete")
	w.WriteHeader(http.StatusOK)
	return
}

func unhandledRequest(w http.ResponseWriter, r *http.Request) {
	log.Printf("\nincoming unhanlded request: %v", r)
}

func makeStringSelectorSafe(s string) string {
	// remove symbols that would break the queryselector
	// this might cause erros
	// TODO: just generate a uuid for the selector or smth
	r := []string{" ", "/", ".", "(", ")"}
	for _, x := range r {
		s = strings.ReplaceAll(s, x, "-")
	}
	return s

}

func (a *App) returnExplorerTemplate(w http.ResponseWriter, r *http.Request) {
	b := r.URL.Query().Get("bucket")
	log.Print(b)
	if b == "" {
		return
	}
	i, err := a.listfilesInBucket(w, b)
	if err != nil {
		log.Panic(err)
		return
	}
	a.bucket = b
	tmpl, err := template.ParseFiles("templates/explorer.html", "templates/folder.html")
	if err != nil {
		log.Panicf("Error creating html template: %v", err)
		return
	}

	err = tmpl.Execute(w, i)
	if err != nil {
		log.Panicf("Error executing html template: %v", err)
		return
	}
}

type Overlay struct {
	Path     string
	Selector string
}

func (a *App) showOverlay(w http.ResponseWriter, r *http.Request) {
	p := r.URL.Query().Get("path")
	o := Overlay{
		Path:     p,
		Selector: makeStringSelectorSafe(p),
	}
	if p == "" {
		o = Overlay{
			Path:     "/",
			Selector: "root",
		}
	}

	tmpl, err := template.ParseFiles("templates/upload-folder.html")
	if err != nil {
		log.Panicf("Error creating html template: %v", err)
		return
	}
	err = tmpl.Execute(w, o)
	if err != nil {
		log.Panicf("Error executing html template: %v", err)
		return
	}
}

func (a *App) createFolder(w http.ResponseWriter, r *http.Request) {
	n := r.FormValue("f-name")
	p := r.URL.Query().Get("path")

	if string(p) == "/" {
		p = ""
	}

	uploader := manager.NewUploader(a.s3Client)
	_, err := uploader.Upload(context.TODO(), &s3.PutObjectInput{
		Bucket: aws.String(a.bucket),
		Key:    aws.String(p + n + "/"),
		Body:   bytes.NewReader([]byte("")),
	})
	if err != nil {
		log.Panicf("Error uploading file:  %v", err)
	}

	tmpl, err := template.ParseFiles("templates/folder.html")
	if err != nil {
		log.Panicf("Error creating html template: %v", err)
		return
	}

	f := `
		{{ template "Folder" .}}
	`
	tmpl, err = tmpl.Parse(f)
	if err != nil {
		log.Panicf("Error creating html template: %v", err)
		return
	}

	fd := Folder{
		Name:     n,
		Selector: makeStringSelectorSafe(p + n + "/"),
		Path:     p + n + "/",
	}
	err = tmpl.Execute(w, fd)
	if err != nil {
		log.Panicf("Error executing html template: %v", err)
		return
	}

}
