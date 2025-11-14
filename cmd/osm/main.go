package main

import (
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
)

const (
	baseUrl = "https://overpass-api.de/api/interpreter"
)

func main() {
	files, err := os.ReadDir("./osm/overpass")
	if err != nil {
		log.Fatal(err)
	}
	for _, file := range files {
		fileName := file.Name()
		overpassFileName := "./osm/overpass/" + fileName
		if filepath.Ext(overpassFileName) != ".overpassql" {
			continue
		}

		query, err := os.ReadFile(overpassFileName)
		if err != nil {
			log.Println("file reading error:", err)
			return
		}

		data, err := getOsmData(string(query))
		if err != nil {
			log.Println("request execution error:", err)
			return
		}

		osmFileName := "./osm/data/" + file.Name()[:len(fileName)-len(filepath.Ext(fileName))] + ".osm"
		saveToFile(osmFileName, data)
	}
}

func getOsmData(query string) ([]byte, error) {
	req, err := http.NewRequest(http.MethodGet, baseUrl, nil)
	if err != nil {
		return nil, err
	}

	q := req.URL.Query()
	q.Add("data", query)
	req.URL.RawQuery = q.Encode()

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}

	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	return body, nil
}

func saveToFile(name string, data []byte) error {
	err := os.WriteFile(name, data, 0644)
	if err != nil {
		return err
	}
	return nil
}
