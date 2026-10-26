package main

import (
	"net/http"
)

func main() {
	newMux := http.NewServeMux()
	newServer := &http.Server{
		Addr: 	 ":8080",
		Handler: newMux,
	}
	newMux.Handle("/", http.FileServer(http.Dir(".")))
	newServer.ListenAndServe()
}