package main

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	"github.com/gorilla/mux"
	_ "github.com/lib/pq"
)

var db *sql.DB

type Product struct {
	ID    int     `json:"id"`
	Name  string  `json:"name"`
	Price float64 `json:"price"`
	Stock int     `json:"stock"`
}

func main() {
	// --- Koneksi & Migrasi Database ---
	connStr := os.Getenv("DATABASE_URL")
	if connStr == "" {
		log.Fatal("DATABASE_URL tidak disetel")
	}

	// runMigrations(connStr)
	initDB(connStr)
	defer db.Close()

	// --- Pengaturan Router & Server ---
	r := mux.NewRouter()
	r.HandleFunc("/products", createProductHandler).Methods("POST")
	r.HandleFunc("/products", getProductsHandler).Methods("GET") // <-- ENDPOINT BARU DITAMBAHKAN
	r.HandleFunc("/products/{id}", getProductHandler).Methods("GET")
	r.HandleFunc("/products/{id}/stock", updateStockHandler).Methods("PUT")

	log.Println("Server berjalan di http://localhost:8080")
	log.Fatal(http.ListenAndServe(":8080", r))
}

// ... (fungsi runMigrations dan initDB tetap sama) ...

func runMigrations(databaseURL string) {
	log.Println("Menjalankan migrasi database...")
	migrationsPath := "file://db/migration"

	m, err := migrate.New(migrationsPath, databaseURL)
	if err != nil {
		log.Fatalf("Gagal membuat instance migrasi: %v", err)
	}

	if err := m.Up(); err != nil && err != migrate.ErrNoChange {
		log.Fatalf("Gagal menjalankan migrasi 'up': %v", err)
	}

	log.Println("Migrasi database berhasil dijalankan.")
}

func initDB(connStr string) {
	var err error
	db, err = sql.Open("postgres", connStr)
	if err != nil {
		log.Fatalf("Gagal membuka koneksi database: %v", err)
	}

	for i := 0; i < 5; i++ {
		err = db.Ping()
		if err == nil {
			log.Println("Berhasil terhubung ke database.")
			return
		}
		log.Printf("Gagal ping database, mencoba lagi dalam 2 detik... (%v)", err)
		time.Sleep(2 * time.Second)
	}
	log.Fatalf("Tidak dapat terhubung ke database setelah beberapa kali percobaan: %v", err)
}

// --- HANDLER BARU UNTUK MENGAMBIL SEMUA PRODUK ---
func getProductsHandler(w http.ResponseWriter, r *http.Request) {
	sqlStatement := `SELECT id, name, price, stock FROM products`
	rows, err := db.Query(sqlStatement)
	if err != nil {
		http.Error(w, "Gagal mengambil daftar produk", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var products []Product
	for rows.Next() {
		var p Product
		if err := rows.Scan(&p.ID, &p.Name, &p.Price, &p.Stock); err != nil {
			http.Error(w, "Gagal memindai data produk", http.StatusInternalServerError)
			return
		}
		products = append(products, p)
	}
	// Periksa error yang mungkin terjadi selama iterasi
	if err = rows.Err(); err != nil {
		http.Error(w, "Error saat iterasi produk", http.StatusInternalServerError)
		return
	}

	// Jika tidak ada produk, kembalikan array JSON kosong, bukan error
	if products == nil {
		products = make([]Product, 0)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(products)
}

// Handler untuk MENULIS produk baru
func createProductHandler(w http.ResponseWriter, r *http.Request) {
	var p Product
	if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	sqlStatement := `INSERT INTO products (name, price, stock) VALUES ($1, $2, $3) RETURNING id`
	err := db.QueryRow(sqlStatement, p.Name, p.Price, p.Stock).Scan(&p.ID)
	if err != nil {
		http.Error(w, "Gagal membuat produk", http.StatusInternalServerError)
		log.Printf("Error QueryRow: %v", err)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(p)
}

// Handler untuk MEMBACA satu produk
func getProductHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id, _ := strconv.Atoi(vars["id"])

	var p Product
	sqlStatement := `SELECT id, name, price, stock FROM products WHERE id=$1`
	err := db.QueryRow(sqlStatement, id).Scan(&p.ID, &p.Name, &p.Price, &p.Stock)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			http.NotFound(w, r)
		} else {
			http.Error(w, "Gagal mengambil produk", http.StatusInternalServerError)
		}
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(p)
}

// Handler untuk MEMPERBARUI stok
func updateStockHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id, _ := strconv.Atoi(vars["id"])

	var payload struct {
		Stock int `json:"stock"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	sqlStatement := `UPDATE products SET stock = $1 WHERE id = $2`
	res, err := db.Exec(sqlStatement, payload.Stock, id)
	if err != nil {
		http.Error(w, "Gagal memperbarui stok", http.StatusInternalServerError)
		return
	}

	rowsAffected, _ := res.RowsAffected()
	if rowsAffected == 0 {
		http.NotFound(w, r)
		return
	}

	w.WriteHeader(http.StatusOK)
	fmt.Fprint(w, "Stok berhasil diperbarui")
}
