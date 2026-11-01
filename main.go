package main

import (
	"net/http"
	"fmt"
)

func main() {
	newMux := http.NewServeMux()
	newServer := &http.Server{
		Addr: 	 ":8080",
		Handler: newMux,
	}
	newMux.Handle("/app/", http.StripPrefix("/app", http.FileServer(http.Dir("."))))
	newMux.HandleFunc("/healthz", readinessEndpoint)
	fmt.Println("Now serving webpage on :8080")
	newServer.ListenAndServe()
}

func readinessEndpoint(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(200)
	w.Write([]byte("OK"))
}