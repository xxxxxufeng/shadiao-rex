package main

import (
	"context"
	"flag"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/xxxxxufeng/shadiao-rex/server/internal/api"
	"github.com/xxxxxufeng/shadiao-rex/server/internal/config"
	"github.com/xxxxxufeng/shadiao-rex/server/internal/history"
	"github.com/xxxxxufeng/shadiao-rex/server/internal/pool"
	"github.com/xxxxxufeng/shadiao-rex/server/internal/upstream"
)

func main() {
	home, _ := os.UserHomeDir()
	dataDir := flag.String("data", filepath.Join(home, "hgsd"), "data directory")
	flag.Parse()
	cfg, err := config.Load(*dataDir)
	if err != nil {
		log.Fatal(err)
	}
	up, err := upstream.NewSDK(cfg.DataDir)
	if err != nil {
		log.Fatal(err)
	}
	hist, err := history.Open(filepath.Join(cfg.DataDir, "history.json"))
	if err != nil {
		log.Fatal(err)
	}
	p := pool.New(up, cfg.Keywords, 3, filepath.Join(cfg.DataDir, "pool.json"))
	go p.Run(context.Background(), time.Duration(cfg.RefreshHours)*time.Hour)

	mux := api.AccessLog(log.Default(), api.NewMux(api.Deps{Cfg: cfg, Up: up, Pool: p, Hist: hist, HTTP: &http.Client{Timeout: 0}}))
	srv := &http.Server{Addr: cfg.Listen, Handler: mux, ReadHeaderTimeout: 10 * time.Second}
	log.Printf("hgsd listening on %s, data %s", cfg.Listen, cfg.DataDir)
	log.Fatal(srv.ListenAndServe())
}
