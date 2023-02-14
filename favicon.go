package main

import (
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"go.deanishe.net/favicon"
)

const iconCacheDir = "icons"
const cacheTryPrefix = "icon-download-"

func ensureIconCache() {
	os.MkdirAll(filepath.Join(wf.CacheDir(), iconCacheDir), 0755)
}

func getIconPath(link Link) string {
	return filepath.Join(wf.CacheDir(), iconCacheDir, link.Short+".png")
}

func getIconCacheKey(link Link) string {
	return fmt.Sprintf("%s%s", cacheTryPrefix, link.Short)
}

func hasIcon(link Link) bool {
	if _, err := os.Stat(getIconPath(link)); err != nil {
		return false
	}

	return true
}

func canTryIconDownload(link Link) bool {
	cacheKey := getIconCacheKey(link)
	// do not try to download this icon again for 24 hours
	return !(wf.Cache.Exists(cacheKey) && !wf.Cache.Expired(cacheKey, time.Hour*24))
}

func (l LinkList) needIconDownload() bool {
	for _, link := range l {
		if hasIcon(link) {
			continue
		}

		if !canTryIconDownload(link) {
			continue
		}

		return true
	}

	return false
}

func (l LinkList) downloadIcons() {
	log.Println("download icons")

	for _, link := range l {
		if hasIcon(link) {
			continue
		}

		if !canTryIconDownload(link) {
			continue
		}

		cacheKey := getIconCacheKey(link)
		if err := downloadIcon(link); err != nil {
			log.Printf("error downloading icon for %s: %s", link.Short, err)
			wf.Cache.Store(cacheKey, []byte(time.Now().String()))
		} else {
			log.Printf("downloaded icon for %s", link.Short)
		}
	}
}

var ErrNoIconFound = errors.New("no icon found")

func downloadIcon(link Link) error {
	icons, err := favicon.Find(link.Long)
	if err != nil {
		return err
	}

	if len(icons) > 0 {
		log.Printf("found %d icons at %s", len(icons), link.Long)

		url := icons[0].URL
		iconPath := getIconPath(link)

		log.Printf("downloading icon for %s from %s", link.Short, url)

		resp, err := http.Get(url)
		if err != nil {
			return err
		}
		defer resp.Body.Close()

		out, err := os.Create(iconPath)
		if err != nil {
			return fmt.Errorf("error creating icon file: %w", err)
		}
		defer func() { _ = out.Close() }()

		_, err = io.Copy(out, resp.Body)
		if err != nil {
			_ = out.Close()
			os.Remove(iconPath)
			return fmt.Errorf("error downloading icon file: %w", err)
		}

		log.Printf("downloaded icon for %s", link.Short)
	} else {
		return ErrNoIconFound
	}

	return nil
}
