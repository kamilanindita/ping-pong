package main

import (
	"context"
	"database/sql"
	"encoding/json" // Library standar
	"errors"
	"log"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/go-redis/redis/v8"
	"github.com/gorilla/mux"
	_ "github.com/lib/pq"

	jsoniter "github.com/json-iterator/go" // Library alternatif yang cepat
)

var (
	db    *sql.DB
	rdb   *redis.Client
	ctx   = context.Background()
	jsoni = jsoniter.ConfigCompatibleWithStandardLibrary // Instance Json-Iterator
)

const (
	cacheKeyStandard = "products_standard"
	cacheKeyIterator = "products_iterator"
)

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

	r := mux.NewRouter()
	// Endpoint untuk perbandingan
	r.HandleFunc("/products-standard", getProductsStandardHandler).Methods("GET")
	r.HandleFunc("/products-iterator", getProductsIteratorHandler).Methods("GET")

	// Endpoint lain (menggunakan jsoniter untuk konsistensi)
	r.HandleFunc("/products", createProductHandler).Methods("POST")
	r.HandleFunc("/products/{id}", getProductHandler).Methods("GET")
	r.HandleFunc("/products/{id}/stock", updateStockHandler).Methods("PUT")

	log.Println("Server berjalan di http://localhost:8080")
	log.Fatal(http.ListenAndServe(":8080", r))
}

// Handler yang menggunakan encoding/json standar
func getProductsStandardHandler(w http.ResponseWriter, r *http.Request) {
	handleGetProducts(w, r, cacheKeyStandard, json.Marshal)
}

// Handler yang menggunakan json-iterator/go
func getProductsIteratorHandler(w http.ResponseWriter, r *http.Request) {
	handleGetProducts(w, r, cacheKeyIterator, jsoni.Marshal)
}

// Fungsi generik untuk logika get products dengan marshaller yang bisa diganti
func handleGetProducts(w http.ResponseWriter, r *http.Request, cacheKey string, marshaller func(v interface{}) ([]byte, error)) {
	cachedProducts, err := rdb.Get(ctx, cacheKey).Result()
	if err == nil {
		log.Printf("CACHE HIT: Mengambil dari Redis untuk kunci %s.", cacheKey)
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(cachedProducts))
		return
	}

	log.Printf("CACHE MISS: Mengambil dari PostgreSQL untuk kunci %s.", cacheKey)
	products, err := fetchProductsFromDB()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Gunakan marshaller yang diberikan (bisa json.Marshal atau jsoni.Marshal)
	jsonData, err := marshaller(products)
	if err != nil {
		http.Error(w, "Gagal mem-format data untuk cache", http.StatusInternalServerError)
		return
	}

	err = rdb.Set(ctx, cacheKey, jsonData, 10*time.Minute).Err()
	if err != nil {
		log.Printf("Gagal menyimpan ke Redis: %v", err)
	}

	w.Header().Set("Content-Type", "application/json")
	w.Write(jsonData)
}

func fetchProductsFromDB() ([]Product, error) {
	sqlStatement := `SELECT id, name, price, stock FROM products limit 50`
	rows, err := db.Query(sqlStatement)
	if err != nil {
		return nil, errors.New("gagal mengambil daftar produk")
	}
	defer rows.Close()

	var products []Product
	for rows.Next() {
		var p Product
		if err := rows.Scan(&p.ID, &p.Name, &p.Price, &p.Stock); err != nil {
			return nil, errors.New("gagal memindai data produk")
		}
		products = append(products, p)
	}
	if err = rows.Err(); err != nil {
		return nil, errors.New("error saat iterasi produk")
	}

	if products == nil {
		products = make([]Product, 0)
	}
	return products, nil
}

// Handler lain sekarang menggunakan jsoniter untuk efisiensi
func createProductHandler(w http.ResponseWriter, r *http.Request) {
	var p Product
	if err := jsoni.NewDecoder(r.Body).Decode(&p); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	sqlStatement := `INSERT INTO products (name, price, stock) VALUES ($1, $2, $3) RETURNING id`
	err := db.QueryRow(sqlStatement, p.Name, p.Price, p.Stock).Scan(&p.ID)
	if err != nil {
		http.Error(w, "Gagal membuat produk", http.StatusInternalServerError)
		return
	}
	// Invalidate kedua cache
	rdb.Del(ctx, cacheKeyStandard, cacheKeyIterator)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	jsoni.NewEncoder(w).Encode(p)
}

// Ditambahkan di sini agar file lengkap
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
	_, err := db.Exec(sqlStatement, payload.Stock, id)
	if err != nil {
		http.Error(w, "Gagal memperbarui stok", http.StatusInternalServerError)
		return
	}
	// Invalidate kedua cache
	rdb.Del(ctx, cacheKeyStandard, cacheKeyIterator)
	w.WriteHeader(http.StatusOK)
}

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
	jsoni.NewEncoder(w).Encode(p)
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

func initRedis(redisURL string) {
	rdb = redis.NewClient(&redis.Options{
		Addr: redisURL,
	})
	if _, err := rdb.Ping(ctx).Result(); err != nil {
		log.Fatalf("Tidak dapat terhubung ke Redis: %v", err)
	}
	log.Println("Berhasil terhubung ke Redis.")
}
