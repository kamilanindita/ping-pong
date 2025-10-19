package main

import (
	"encoding/json"
	"log"
	"net/http"
)

// main adalah fungsi utama yang akan dieksekusi.
func main() {
	// Mendaftarkan handler untuk endpoint "/ping".
	// Setiap request ke "/ping" akan ditangani oleh fungsi ini.
	http.HandleFunc("/ping", func(w http.ResponseWriter, r *http.Request) {
		// Mengatur header Content-Type agar klien tahu bahwa responsnya adalah JSON.
		w.Header().Set("Content-Type", "application/json")

		// Membuat data respons dalam bentuk map.
		response := map[string]string{"message": "pong"}

		// Meng-encode map menjadi format JSON dan mengirimkannya sebagai respons.
		err := json.NewEncoder(w).Encode(response)
		if err != nil {
			// Jika terjadi error saat encoding JSON, catat error dan kirim status 500.
			log.Printf("Error encoding JSON response: %v", err)
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		}
	})

	// Memberi tahu di console bahwa server akan dimulai.
	log.Println("Server Go berjalan di http://localhost:8080")

	// Memulai server HTTP di port 8080.
	// Jika server gagal dimulai, program akan berhenti dengan pesan error.
	if err := http.ListenAndServe(":8080", nil); err != nil {
		log.Fatalf("Tidak dapat memulai server: %s\n", err)
	}
}
