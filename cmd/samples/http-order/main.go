package main

import (
	"log"
	"net/http"
)

func main() {
	http.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"healthy","service":"order"}`))
	})

	http.HandleFunc("/orders", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`[{"id":101,"user_id":1,"amount":99.99},{"id":102,"user_id":2,"amount":149.50}]`))
	})

	http.HandleFunc("/orders/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"id":101,"user_id":1,"amount":99.99,"status":"completed"}`))
	})

	log.Println("Order service listening on :9002")
	if err := http.ListenAndServe(":9002", nil); err != nil {
		log.Fatal(err)
	}
}
