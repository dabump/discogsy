package collection

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type Record struct {
	DiscogsID int    `json:"discogs id"`
	Artist    string `json:"artist"`
	Album     string `json:"album"`
	Year      *int   `json:"year"`
	Link      string `json:"discogs link"`
	Poster    string `json:"poster"`
}

func Load(path string) ([]Record, error) {
	cleanPath := filepath.Clean(path)
	file, err := os.Open(cleanPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			if err := os.WriteFile(cleanPath, []byte("[]\n"), 0644); err != nil {
				return nil, err
			}
			return []Record{}, nil
		}
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
		artistI := strings.ToLower(records[i].Artist)
		artistJ := strings.ToLower(records[j].Artist)
		if artistI != artistJ {
			return artistI < artistJ
		}
		albumI := strings.ToLower(records[i].Album)
		albumJ := strings.ToLower(records[j].Album)
		if albumI != albumJ {
			return albumI < albumJ
		}
		return records[i].DiscogsID < records[j].DiscogsID
	})
}
