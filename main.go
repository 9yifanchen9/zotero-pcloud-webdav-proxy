package main

import (
	"log"
	"net/http"
	"os"
)

func main() {
	cfg, err := loadConfig(os.Getenv)
	if err != nil {
		log.Fatal(err)
	}

	proxy := NewReverseProxy(cfg.pcloudUser, cfg.pcloudPass, cfg.pcloudURL)
	handler := requireAuth(cfg.proxyUser, cfg.proxyPass, proxy)
	http.Handle("/", handler)

	log.Println("Begin listening")

	err = http.ListenAndServe(":8080", nil)
	if err != nil {
		log.Fatal(err)
	}

}
