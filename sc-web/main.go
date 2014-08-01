package main

import (
	"fmt"
	"log"
	"net/http"
)

func main() {
	hdl := http.FileServer(http.Dir("/Users/xlab/Documents/Coding/Web/smscloud.org"))
	http.Handle("/", hdl)
	fmt.Printf("Serving at http://localhost:5051\n")
	if err := http.ListenAndServe(":5051", nil); err != nil {
		log.Fatalln(err)
	}
}
