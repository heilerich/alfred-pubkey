package main

import (
	"bufio"
	"bytes"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"time"

	aw "github.com/deanishe/awgo"
	"golang.org/x/crypto/ssh"
)

var (
	wf           *aw.Workflow
	doDownload   bool
	forceRefresh bool
)

func init() {
	wf = aw.New()
	flag.BoolVar(&doDownload, "download", false, "Download keys")
	flag.BoolVar(&forceRefresh, "refresh", false, "Force download keys")
}

func run() {
	query := wf.Args()[0]
	flag.Parse()

	if doDownload {
		wf.Configure(aw.TextErrors(true))
		log.Printf("Background job")
		if _, err := downloadFile(); err != nil {
			wf.FatalError(fmt.Errorf("error downloading links: %w", err))
		}
	}

	keys, err := getKeys()
	if err != nil {
		wf.FatalError(fmt.Errorf("error getting links: %w", err))
	}

	for _, key := range keys {
		_ = wf.NewItem(key.KeyLine).
			Autocomplete(key.Comment).
			Title(key.Comment).
			Subtitle(key.KeyLine).
			Arg(key.KeyLine).
			Valid(true)
	}

	_ = wf.NewItem("Refresh").
		Autocomplete("refresh").
		Title("Refresh").
		Subtitle("Force refresh of keys").
		Arg("refresh").
		Valid(true)

	if query != "" && len(keys) > 0 {
		wf.Filter(query)
	}

	if !wf.IsRunning("download") && wf.IsEmpty() {
		wf.NewItem("No keys found").
			Valid(false)
	}

	wf.SendFeedback()
}

func main() {
	wf.Run(run)
}

const keysCacheKey = "pubkey-cache"

type pubKey struct {
	KeyLine string
	Comment string
}

type keyList []pubKey

func downloadFile() (keyList, error) {
	log.Printf("Downloading links")

	resp, err := http.Get("https://git.io/heilek")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	keys := keyList{}

	scanner := bufio.NewScanner(resp.Body)
	buffer := bytes.NewBuffer([]byte{})

	for scanner.Scan() {
		keyBytes := scanner.Bytes()
		_, _ = buffer.Write(keyBytes)
		_, _ = buffer.Write([]byte("\n"))

		for len(keyBytes) > 0 {
			key, comment, _, rest, err := ssh.ParseAuthorizedKey(keyBytes)
			keyBytes = rest
			if err != nil {
				wf.FatalError(fmt.Errorf("error parsing key file: %w", err))
			}

			keyString := string(ssh.MarshalAuthorizedKey(key))
			if len(keyString) > 0 {
				keys = append(keys, pubKey{
					KeyLine: fmt.Sprintf("%s %s", keyString[:len(keyString)-1], comment),
					Comment: comment,
				})
			}
		}
	}

	completeString := buffer.String()
	if len(completeString) > 0 {
		keys = append(keyList{pubKey{
			KeyLine: completeString[:len(completeString)-1],
			Comment: "All keys",
		}}, keys...)
	}

	if err := scanner.Err(); err != nil {
		wf.FatalError(fmt.Errorf("error scanning response: %w", err))
	}

	log.Printf("Downloaded %d pubkeys", len(keys))

	if err = wf.Data.StoreJSON(keysCacheKey, keys); err != nil {
		return nil, err
	}

	log.Printf("persisted keys to cache")

	return keys, nil
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

func getKeys() (keyList, error) {
	log.Printf("Getting keys")

	if !wf.Data.Exists(keysCacheKey) {
		log.Printf("Empty cache, downloading")
		wf.NewItem("Refreshing data...").Valid(false)
		backgroundDownload()
		return keyList{}, nil
	}

	if wf.Data.Expired(keysCacheKey, 24*time.Hour) {
		log.Printf("Cache expired, refreshing in background")
		backgroundDownload()
	}

	if forceRefresh {
		log.Printf("Forcing refresh")
		backgroundDownload()
	}

	var keys keyList
	if err := wf.Data.LoadJSON(keysCacheKey, &keys); err != nil {
		return nil, err
	}

	return keys, nil
}
