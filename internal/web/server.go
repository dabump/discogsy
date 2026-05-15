package web

import (
	"encoding/json"
	"html/template"
	"math/rand"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/dabump/discogsy/internal/collection"
)

type pageData struct {
	RecordsJSON template.JS
	HeroPosters []string
}

type RecordStore struct {
	mu      sync.RWMutex
	records []collection.Record
}

func NewRecordStore(records []collection.Record) *RecordStore {
	collection.Sort(records)
	return &RecordStore{records: records}
}

func (s *RecordStore) Records() []collection.Record {
	s.mu.RLock()
	defer s.mu.RUnlock()

	records := make([]collection.Record, len(s.records))
	copy(records, s.records)
	return records
}

func (s *RecordStore) SetRecords(records []collection.Record) {
	collection.Sort(records)

	s.mu.Lock()
	defer s.mu.Unlock()
	s.records = records
}

func NewHandler(store *RecordStore, posterDirs []string, templatePath string) http.Handler {
	page := template.Must(template.ParseFiles(templatePath))
	mux := http.NewServeMux()
	mux.Handle("/posters/", http.StripPrefix("/posters/", posterFileServer(posterDirs)))
	mux.HandleFunc("/api/records", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		if err := json.NewEncoder(w).Encode(store.Records()); err != nil {
			http.Error(w, "failed to encode records", http.StatusInternalServerError)
		}
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}

		records := store.Records()
		recordsJSON, err := json.Marshal(records)
		if err != nil {
			http.Error(w, "failed to encode records", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		if err := page.Execute(w, pageData{RecordsJSON: template.JS(recordsJSON), HeroPosters: randomPosters(records, 28)}); err != nil {
			http.Error(w, "failed to render page", http.StatusInternalServerError)
		}
	})

	return mux
}

func posterFileServer(dirs []string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		name := filepath.Base(filepath.Clean(r.URL.Path))
		if name == "." || name == string(filepath.Separator) {
			http.NotFound(w, r)
			return
		}
		for _, dir := range dirs {
			path := filepath.Join(filepath.Clean(dir), name)
			if info, err := os.Stat(path); err == nil && !info.IsDir() {
				http.ServeFile(w, r, path)
				return
			}
		}
		http.NotFound(w, r)
	})
}

func randomPosters(records []collection.Record, limit int) []string {
	posters := make([]string, 0, len(records))
	seen := make(map[string]bool, len(records))
	for _, record := range records {
		if record.Poster == "" || seen[record.Poster] {
			continue
		}
		seen[record.Poster] = true
		posters = append(posters, record.Poster)
	}

	rand.New(rand.NewSource(time.Now().UnixNano())).Shuffle(len(posters), func(i, j int) {
		posters[i], posters[j] = posters[j], posters[i]
	})
	if len(posters) > limit {
		posters = posters[:limit]
	}
	return posters
}
