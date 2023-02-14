package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"time"

	aw "github.com/deanishe/awgo"
)

var (
	wf         *aw.Workflow
	doDownload bool
)

func init() {
	wf = aw.New()
	flag.BoolVar(&doDownload, "download", false, "Download links")
}

func run() {
	query := wf.Args()[0]
	flag.Parse()

	ensureIconCache()

	if doDownload {
		wf.Configure(aw.TextErrors(true))
		log.Printf("Background job")
		if _, err := downloadLinks(); err != nil {
			wf.FatalError(fmt.Errorf("error downloading links: %w", err))
		}
	}

	splits := strings.SplitN(query, "/", 2)
	search := splits[0]
	path := ""

	if len(splits) == 2 {
		path = splits[1]
	}

	links, err := getLinks()
	if err != nil {
		wf.FatalError(fmt.Errorf("error getting links: %w", err))
	}

	for _, link := range links {
		item := wf.NewItem(link.Long).
			Autocomplete(fmt.Sprintf("%s/%s", link.Short, path)).
			Title(fmt.Sprintf("%s (%s)", link.Short, link.Owner)).
			Subtitle(link.Long).
			Arg(fmt.Sprintf("http://go/%s/%s", link.Short, path)).
			Match(fmt.Sprintf("%s %s", link.Short, link.Long)).
			Valid(true)

		if hasIcon(link) {
			item.Icon(&aw.Icon{Value: getIconPath(link)})
		}

		item.Alt().
			Subtitle("Go to detail page.").
			Arg(fmt.Sprintf("http://go/.detail/%s", link.Short)).
			Valid(true)
	}

	if search != "" && len(links) > 0 {
		wf.Filter(search)
	}

	if !wf.IsRunning("download") && wf.IsEmpty() {
		wf.NewItem("No links found").
			Subtitle("Press enter to go to golink home page").
			Arg("http://go").
			Valid(true)
	}

	wf.SendFeedback()
}

func main() {
	wf.Run(run)
}

const linkCacheKey = "linkCache"

type Link struct {
	Short    string
	Long     string
	Created  time.Time
	LastEdit time.Time
	Owner    string
}

type LinkList []Link

func downloadLinks() (LinkList, error) {
	log.Printf("Downloading links")

	resp, err := http.Get("http://go/.export")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	links := LinkList{}

	scanner := bufio.NewScanner(resp.Body)
	for scanner.Scan() {
		var link Link

		if err := json.Unmarshal(scanner.Bytes(), &link); err != nil {
			return nil, err
		}

		links = append(links, link)
	}

	if err := scanner.Err(); err != nil {
		wf.FatalError(fmt.Errorf("error scanning response: %w", err))
	}

	log.Printf("Downloaded %d links", len(links))

	if err = wf.Data.StoreJSON(linkCacheKey, links); err != nil {
		return nil, err
	}

	log.Printf("persisted links to cache")

	if links.needIconDownload() {
		log.Println("some links need icons")
		links.downloadIcons()
	}

	return links, nil
}

func backgroundDownload() {
	wf.Rerun(0.2)
	if !wf.IsRunning("download") {
		log.Printf("starting background job")
		cmd := exec.Command(os.Args[0], "-download")
		if err := wf.RunInBackground("download", cmd); err != nil {
			wf.FatalError(fmt.Errorf("error running background job: %w", err))
		}
	} else {
		log.Printf("Background job already running")
	}
}

func getLinks() (LinkList, error) {
	log.Printf("Getting links")

	if !wf.Data.Exists(linkCacheKey) {
		log.Printf("Empty cache, downloading")
		wf.NewItem("Refreshing data...").Valid(false)
		backgroundDownload()
		return []Link{}, nil
	}

	if wf.Data.Expired(linkCacheKey, 5*time.Second) {
		log.Printf("Cache expired, refreshing in background")
		backgroundDownload()
	}

	var links []Link
	if err := wf.Data.LoadJSON(linkCacheKey, &links); err != nil {
		return nil, err
	}

	return links, nil
}
