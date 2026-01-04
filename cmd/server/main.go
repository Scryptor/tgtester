package main

import (
	"flag"
	"log"
	"net/http"

	"SendMsgTestForTG/internal/server"
)

func main() {
	addr := flag.String("addr", ":8080", "Адрес для прослушивания")
	flag.Parse()

	srv := server.NewServer()
	srv.StartLogBroadcaster()

	http.HandleFunc("/api/config", srv.GetConfig)
	http.HandleFunc("/api/config/update", srv.UpdateConfig)
	http.HandleFunc("/api/start", srv.Start)
	http.HandleFunc("/api/stop", srv.Stop)
	http.HandleFunc("/api/status", srv.GetStatus)
	http.HandleFunc("/api/logs", srv.LogsSSE)
	http.Handle("/", http.FileServer(http.Dir("./web/static")))

	log.Printf("Сервер запущен на http://localhost%s", *addr)
	if err := http.ListenAndServe(*addr, nil); err != nil {
		log.Fatal(err)
	}
}

