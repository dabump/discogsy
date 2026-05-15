package collection

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type Record struct {
	Artist string `json:"artist"`
	Album  string `json:"album"`
	Year   *int   `json:"year"`
	Link   string `json:"discogs link"`
	Poster string `json:"poster"`
}

func Load(path string) ([]Record, error) {
	file, err := os.Open(filepath.Clean(path))
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var records []Record
	if err := json.NewDecoder(file).Decode(&records); err != nil {
		return nil, err
	}
	return records, nil
}

func Save(path string, records []Record) error {
	Sort(records)

	var data bytes.Buffer
	encoder := json.NewEncoder(&data)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(records); err != nil {
		return err
	}
	return os.WriteFile(filepath.Clean(path), data.Bytes(), 0644)
}

func Sort(records []Record) {
	sort.Slice(records, func(i, j int) bool {
		return strings.ToLower(records[i].Artist) < strings.ToLower(records[j].Artist)
	})
}
