package discogs

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"mime"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/dabump/discogsy/internal/collection"
)

const apiBaseURL = "https://api.discogs.com"

type Config struct {
	Username string
	Token    string
}

type Store interface {
	SetRecords([]collection.Record)
}

type Client struct {
	config Config
	http   *http.Client
}

type collectionResponse struct {
	Pagination struct {
		Page  int `json:"page"`
		Pages int `json:"pages"`
	} `json:"pagination"`
	Releases []collectionRelease `json:"releases"`
}

type collectionRelease struct {
	ID               int              `json:"id"`
	InstanceID       int              `json:"instance_id"`
	BasicInformation basicInformation `json:"basic_information"`
}

type basicInformation struct {
	ID         int      `json:"id"`
	Title      string   `json:"title"`
	Year       int      `json:"year"`
	CoverImage string   `json:"cover_image"`
	Thumb      string   `json:"thumb"`
	Artists    []artist `json:"artists"`
}

type artist struct {
	Name string `json:"name"`
}

func ConfigFromEnv() (Config, error) {
	config := Config{
		Username: envValue("DISCOGS_USERNAME", "USERNAME"),
		Token:    envValue("DISCOGS_TOKEN", "TOKEN"),
	}

	var missing []string
	if config.Username == "" {
		missing = append(missing, "DISCOGS_USERNAME or USERNAME")
	}
	if config.Token == "" {
		missing = append(missing, "DISCOGS_TOKEN or TOKEN")
	}
	if len(missing) > 0 {
		return Config{}, fmt.Errorf("missing required environment variable(s): %s", strings.Join(missing, ", "))
	}
	return config, nil
}

func envValue(names ...string) string {
	for _, name := range names {
		if value := strings.TrimSpace(os.Getenv(name)); value != "" {
			return value
		}
	}
	return ""
}

func NewClient(config Config) *Client {
	return &Client{
		config: config,
		http: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

func (c *Client) Sync(collectionPath string, posterDir string) ([]collection.Record, bool, error) {
	previousRecords, err := collection.Load(collectionPath)
	if err != nil {
		return nil, false, err
	}

	previousByID := make(map[int]collection.Record, len(previousRecords))
	previousByLink := make(map[string]collection.Record, len(previousRecords))
	for _, record := range previousRecords {
		if record.DiscogsID != 0 {
			previousByID[record.DiscogsID] = record
		}
		if record.Link != "" {
			previousByLink[record.Link] = record
		}
	}

	releases, err := c.collectionReleases()
	if err != nil {
		return nil, false, err
	}

	records := make([]collection.Record, 0, len(releases))
	for _, release := range releases {
		info := release.BasicInformation
		if info.ID == 0 {
			continue
		}

		link := fmt.Sprintf("https://www.discogs.com/release/%d", info.ID)
		discogsID := release.InstanceID
		if discogsID == 0 {
			discogsID = release.ID
		}
		if discogsID == 0 {
			discogsID = info.ID
		}

		previous := previousByID[discogsID]
		if previous.Poster == "" {
			previous = previousByLink[link]
		}

		poster := previous.Poster
		if poster == "" {
			var err error
			poster, err = c.downloadPoster(info, posterDir)
			if err != nil {
				return nil, false, fmt.Errorf("download poster for release %d: %w", info.ID, err)
			}
		}

		records = append(records, collection.Record{
			DiscogsID: discogsID,
			Artist:    artistNames(info.Artists),
			Album:     info.Title,
			Year:      yearPtr(info.Year),
			Link:      link,
			Poster:    poster,
		})
	}

	changed := !sameRecords(previousRecords, records)
	if !changed {
		return records, false, nil
	}
	if err := collection.Save(collectionPath, records); err != nil {
		return nil, false, err
	}
	return records, true, nil
}

func (c *Client) collectionReleases() ([]collectionRelease, error) {
	var releases []collectionRelease
	for page := 1; ; page++ {
		endpoint := fmt.Sprintf("%s/users/%s/collection/folders/0/releases?page=%d&per_page=100", apiBaseURL, url.PathEscape(c.config.Username), page)
		var response collectionResponse
		if err := c.getJSON(endpoint, &response); err != nil {
			return nil, fmt.Errorf("get collection page %d for Discogs username %q: %w", page, c.config.Username, err)
		}
		releases = append(releases, response.Releases...)
		if response.Pagination.Pages == 0 || page >= response.Pagination.Pages {
			break
		}
	}
	return releases, nil
}

func (c *Client) getJSON(endpoint string, target any) error {
	request, err := http.NewRequest(http.MethodGet, endpoint, nil)
	if err != nil {
		return err
	}
	c.authorize(request)

	response, err := c.http.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()

	if response.StatusCode < 200 || response.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(response.Body, 1024))
		return fmt.Errorf("discogs api returned %s: %s", response.Status, strings.TrimSpace(string(body)))
	}
	return json.NewDecoder(response.Body).Decode(target)
}

func (c *Client) downloadPoster(info basicInformation, posterDir string) (string, error) {
	imageURL := info.CoverImage
	if imageURL == "" {
		imageURL = info.Thumb
	}
	if imageURL == "" {
		return "", fmt.Errorf("no cover image available")
	}

	request, err := http.NewRequest(http.MethodGet, imageURL, nil)
	if err != nil {
		return "", err
	}
	c.authorize(request)

	response, err := c.http.Do(request)
	if err != nil {
		return "", err
	}
	defer response.Body.Close()

	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return "", fmt.Errorf("image request returned %s", response.Status)
	}

	filename := posterFilename(info, response.Header.Get("Content-Type"), imageURL)
	if err := os.MkdirAll(filepath.Clean(posterDir), 0o755); err != nil {
		return "", err
	}

	file, err := os.Create(filepath.Join(filepath.Clean(posterDir), filename))
	if err != nil {
		return "", err
	}
	defer file.Close()

	if _, err := io.Copy(file, response.Body); err != nil {
		return "", err
	}
	return filename, nil
}

func (c *Client) authorize(request *http.Request) {
	request.Header.Set("Authorization", "Discogs token="+c.config.Token)
	request.Header.Set("User-Agent", "discogsy/1.0")
}

func RunEvery(interval time.Duration, client *Client, collectionPath string, posterDir string, store Store) {
	for {
		fmt.Println("Starting discogs sync...")
		records, changed, err := client.Sync(collectionPath, posterDir)
		fmt.Printf("Changes found in discogos: %v\n", changed)
		if err != nil {
			log.Printf("discogs sync failed: %v", err)
		} else if changed {
			store.SetRecords(records)
			log.Printf("discogs sync added new records")
		}
		fmt.Printf("Sync completed - Interval: %v\n", interval)
		time.Sleep(interval)
	}
}

func artistNames(artists []artist) string {
	names := make([]string, 0, len(artists))
	for _, artist := range artists {
		name := strings.TrimSpace(artist.Name)
		if name != "" {
			names = append(names, name)
		}
	}
	if len(names) == 0 {
		return "Unknown Artist"
	}
	return strings.Join(names, ", ")
}

func yearPtr(year int) *int {
	if year == 0 {
		return nil
	}
	return &year
}

func sameRecord(a collection.Record, b collection.Record) bool {
	return a.DiscogsID == b.DiscogsID &&
		a.Artist == b.Artist &&
		a.Album == b.Album &&
		sameYear(a.Year, b.Year) &&
		a.Link == b.Link &&
		a.Poster == b.Poster
}

func sameRecords(a []collection.Record, b []collection.Record) bool {
	if len(a) != len(b) {
		return false
	}

	a = append([]collection.Record(nil), a...)
	b = append([]collection.Record(nil), b...)
	collection.Sort(a)
	collection.Sort(b)
	for i := range a {
		if !sameRecord(a[i], b[i]) {
			return false
		}
	}
	return true
}

func sameYear(a *int, b *int) bool {
	if a == nil || b == nil {
		return a == b
	}
	return *a == *b
}

func posterFilename(info basicInformation, contentType string, imageURL string) string {
	extension := ".jpeg"
	if mediaType, _, err := mime.ParseMediaType(contentType); err == nil {
		if extensions, err := mime.ExtensionsByType(mediaType); err == nil && len(extensions) > 0 {
			extension = extensions[0]
		}
	}
	if parsed, err := url.Parse(imageURL); err == nil {
		if ext := path.Ext(parsed.Path); ext != "" {
			extension = ext
		}
	}
	return fmt.Sprintf("%d-%s%s", info.ID, slug(info.Title), extension)
}

var nonSlugCharacter = regexp.MustCompile(`[^a-z0-9]+`)

func slug(value string) string {
	result := strings.Trim(nonSlugCharacter.ReplaceAllString(strings.ToLower(value), "-"), "-")
	if result == "" {
		return "release"
	}
	return result
}
