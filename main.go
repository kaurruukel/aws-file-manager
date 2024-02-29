package main

import (
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
	mux.HandleFunc("/home", app.handleGetRoot)
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
	i, err := a.listfilesInBucket(w)
	if err != nil {
		log.Panic(err)
		return

	}
	template, err := template.New("index.html").ParseFiles("templates/index.html", "templates/folder.html")
	if err != nil {
		log.Panicf("Error creating html template: %v", err)
		return
	}
	err = template.ExecuteTemplate(w, "index.html", i)
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

	// _, err = a.s3Client.PutObject(context.TODO(), &s3.PutObjectInput{
	// 	Bucket: aws.String("testing-for-golang"),
	// 	Key:    aws.String(k + handler.Filename),
	// 	Body:   file,
	// })
	// if err != nil {
	// 	log.Printf("failed to upload to %v because %v", k, err)
	// }

	fmt.Printf("\n creating file: %v", k+handler.Filename)
	fmt.Printf("\n key: %v", k)

	uploader := manager.NewUploader(a.s3Client)
	_, err = uploader.Upload(context.TODO(), &s3.PutObjectInput{
		Bucket: aws.String("testing-for-golang"), // TODO: retrieve bucket name from user inp
		Key:    aws.String(k + handler.Filename),
		Body:   file,
	})
	if err != nil {
		log.Panicf("Error uploading file:  %v", err)
	}

	return
}

func (a *App) downloadHandler(w http.ResponseWriter, r *http.Request) {
	k := r.URL.Query().Get("key")
	result, err := a.s3Client.GetObject(context.TODO(), &s3.GetObjectInput{
		Bucket: aws.String("testing-for-golang"),
		Key:    aws.String(k),
	})
	if err != nil {
		log.Printf("Couldn't GetObject: %s", err)
	}
	defer result.Body.Close()
	s := strings.Split(k, "/")

	w.Header().Set("Content-Disposition", "attachment; filename="+s[len(s)-1])
	w.Header().Set("Content-Type", "application/octet-stream")

	if _, err := io.Copy(w, result.Body); err != nil {
		log.Printf("Couldn't write response. Here's why: %v\n", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}

	return
}

func (a *App) listfilesInBucket(w http.ResponseWriter) (*Folder, error) {
	bucket := "testing-for-golang"
	output, err := a.s3Client.ListObjectsV2(context.TODO(),
		&s3.ListObjectsV2Input{
			Bucket: aws.String(bucket),
		},
	)
	if err != nil {
		log.Panicf("problem: %v", err)
	}
	f := Folder{
		Name: "root",
		Path: "/",
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

	_, err := a.s3Client.DeleteObject(context.TODO(), &s3.DeleteObjectInput{
		Bucket: aws.String("testing-for-golang"),
		Key:    aws.String(p),
	})
	if err != nil {
		log.Printf("error deleting obj %v: %v", p, err)
	}

	w.Header().Set("HX-Trigger", "delete")
	w.WriteHeader(http.StatusOK)
	return
}

func unhandledRequest(w http.ResponseWriter, r *http.Request) {
	log.Printf("\nincoming unhanlded request: %v", r)
}

func makeStringSelectorSafe(s string) string {
	// we remove . / and whitespace
	s = strings.ReplaceAll(s, " ", "")
	s = strings.ReplaceAll(s, "/", "-")
	s = strings.ReplaceAll(s, ".", "-")
	return s

}
