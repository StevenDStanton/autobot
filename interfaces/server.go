package interfaces

import (
	"fmt"
	"net/http"
)

func handler(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	fmt.Println("Hello from handlers.Handler()")
}

func StartServer() {
	http.HandleFunc("/", handler)
	err := http.ListenAndServe(":8080", nil)

	if err != nil {
		panic(err)
	}
}
