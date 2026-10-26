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
	newMux.Handle("/", http.FileServer(http.Dir(".")))
	fmt.Println("Now serving webpage on :8080")
	newServer.ListenAndServe()
}