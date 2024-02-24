package main

import (
	"context"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"strconv"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

type Folder struct {
	Name    string
	Files   []File
	Folders []Folder
}

type File struct {
	Name string
	Size int64
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
				Name: name,
			}
			f.Folders, f.Files = addObject(path, newFolder, f.Folders, f.Files)
		} else {
			newFile := File{
				Name: name,
				Size: *o.Size,
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
			files = []File{v}
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
