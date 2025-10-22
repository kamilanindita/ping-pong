package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/go-redis/redis/v8"
	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	"github.com/gorilla/mux"
	_ "github.com/lib/pq"
)

var (
	db  *sql.DB
	rdb *redis.Client
	ctx = context.Background()
)

const cacheKeyProducts = "products"

type Product struct {
	ID    int     `json:"id"`
	Name  string  `json:"name"`
	Price float64 `json:"price"`
	Stock int     `json:"stock"`
}

func main() {
	dbConnStr := os.Getenv("DATABASE_URL")
	redisURL := os.Getenv("REDIS_URL")

	if dbConnStr == "" || redisURL == "" {
		log.Fatal("DATABASE_URL atau REDIS_URL tidak disetel")
	}

	initDB(dbConnStr)
	initRedis(redisURL)
	defer db.Close()

	// (Opsional) Mengaktifkan kembali migrasi jika diperlukan
	// runMigrations(dbConnStr)

	r := mux.NewRouter()
	r.HandleFunc("/products", createProductHandler).Methods("POST")
	r.HandleFunc("/products", getProductsHandler).Methods("GET")
	r.HandleFunc("/products/{id}", getProductHandler).Methods("GET")
	r.HandleFunc("/products/{id}/stock", updateStockHandler).Methods("PUT")

	log.Println("Server berjalan di http://localhost:8080")
	log.Fatal(http.ListenAndServe(":8080", r))
}

func initRedis(redisURL string) {
	rdb = redis.NewClient(&redis.Options{
		Addr: redisURL,
	})

	if _, err := rdb.Ping(ctx).Result(); err != nil {
		log.Fatalf("Tidak dapat terhubung ke Redis: %v", err)
	}
	log.Println("Berhasil terhubung ke Redis.")
}

// Handler GET /products sekarang dengan logika caching
func getProductsHandler(w http.ResponseWriter, r *http.Request) {
	// 1. Coba ambil dari Cache terlebih dahulu
	cachedProducts, err := rdb.Get(ctx, cacheKeyProducts).Result()
	if err == nil {
		// Cache HIT
		log.Println("CACHE HIT: Mengambil produk dari Redis.")
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(cachedProducts))
		return
	}

	if err != redis.Nil {
		log.Printf("Error mengambil dari Redis: %v", err)
	}

	// 2. Jika Cache MISS, ambil dari Database
	log.Println("CACHE MISS: Mengambil produk dari PostgreSQL.")
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
	if err = rows.Err(); err != nil {
		http.Error(w, "Error saat iterasi produk", http.StatusInternalServerError)
		return
	}

	if products == nil {
		products = make([]Product, 0)
	}

	// 3. Simpan hasil dari database ke Cache untuk permintaan berikutnya
	jsonData, err := json.Marshal(products)
	if err != nil {
		http.Error(w, "Gagal mem-format data untuk cache", http.StatusInternalServerError)
		return
	}
	// Tetapkan cache dengan masa berlaku (misalnya, 10 menit)
	err = rdb.Set(ctx, cacheKeyProducts, jsonData, 10*time.Minute).Err()
	if err != nil {
		log.Printf("Gagal menyimpan ke Redis: %v", err)
	}

	w.Header().Set("Content-Type", "application/json")
	w.Write(jsonData)
}

// createProductHandler sekarang menghapus cache
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

	// CACHE INVALIDATION: Hapus cache 'products'
	log.Println("CACHE INVALIDATION: Menghapus kunci 'products'.")
	rdb.Del(ctx, cacheKeyProducts)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(p)
}

// updateStockHandler sekarang menghapus cache
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

	// CACHE INVALIDATION: Hapus cache 'products'
	log.Println("CACHE INVALIDATION: Menghapus kunci 'products'.")
	rdb.Del(ctx, cacheKeyProducts)

	w.WriteHeader(http.StatusOK)
	fmt.Fprint(w, "Stok berhasil diperbarui")
}

// Fungsi lainnya (getProductHandler untuk satu produk, initDB, dll. tetap sama)

func getProductHandler(w http.ResponseWriter, r *http.Request) {
	// Note: Caching untuk item tunggal bisa ditambahkan di sini dengan pola yang sama
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

// Fungsi ini sengaja ditinggalkan kosong karena dinonaktifkan di main()
// Anda bisa mengaktifkannya kembali jika perlu
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
