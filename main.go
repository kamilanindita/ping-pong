package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"time"
)

// --- Interfaces mendefinisikan KONTRAK untuk dependensi eksternal ---

type InventoryChecker interface {
	CheckStock(productID string, quantity int) error
}

type PaymentProcessor interface {
	ProcessPayment(userID string, amount float64) (string, error) // Returns transactionID or error
}

type Notifier interface {
	SendOrderConfirmation(userID string, orderDetails map[string]interface{}) error
}

// --- Implementasi NYATA (yang akan digunakan di produksi) ---

type LiveInventoryService struct {
	// Di dunia nyata, ini akan memiliki URL, http.Client, dll.
}

func (s *LiveInventoryService) CheckStock(productID string, quantity int) error {
	log.Printf("NYATA: Menghubungi Layanan Inventaris untuk produk %s...", productID)
	// Logika panggilan HTTP ke layanan inventaris...
	if productID == "OUTOFSTOCK" {
		return errors.New("stok produk tidak mencukupi")
	}
	return nil
}

type LivePaymentGateway struct{}

func (g *LivePaymentGateway) ProcessPayment(userID string, amount float64) (string, error) {
	log.Printf("NYATA: Memproses pembayaran sebesar %.2f untuk pengguna %s...", amount, userID)
	// Logika panggilan HTTP ke gateway pembayaran...
	if amount > 1000 {
		return "", errors.New("pembayaran ditolak oleh bank")
	}
	return fmt.Sprintf("txn_%d", time.Now().UnixNano()), nil
}

type LiveEmailNotifier struct{}

func (n *LiveEmailNotifier) SendOrderConfirmation(userID string, orderDetails map[string]interface{}) error {
	log.Printf("NYATA: Mengirim email konfirmasi ke %s...", userID)
	// Logika panggilan HTTP ke layanan email...
	return nil
}

// --- Handler Utama yang berisi Logika Alur Kerja ---

type OrderHandler struct {
	inventorySvc InventoryChecker
	paymentSvc   PaymentProcessor
	notifierSvc  Notifier
}

// NewOrderHandler adalah constructor yang menerima dependensi (Dependency Injection)
func NewOrderHandler(inv InventoryChecker, pay PaymentProcessor, notif Notifier) *OrderHandler {
	return &OrderHandler{
		inventorySvc: inv,
		paymentSvc:   pay,
		notifierSvc:  notif,
	}
}

func (h *OrderHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	var orderData map[string]interface{}
	if err := json.NewDecoder(r.Body).Decode(&orderData); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	productID := orderData["product_id"].(string)
	quantity := int(orderData["quantity"].(float64))
	amount := orderData["amount"].(float64)
	userID := orderData["user_id"].(string)

	// Langkah 1: Cek Stok
	if err := h.inventorySvc.CheckStock(productID, quantity); err != nil {
		http.Error(w, fmt.Sprintf("Gagal memeriksa stok: %v", err), http.StatusConflict) // 409 Conflict
		return
	}

	// Langkah 2: Proses Pembayaran
	transactionID, err := h.paymentSvc.ProcessPayment(userID, amount)
	if err != nil {
		http.Error(w, fmt.Sprintf("Gagal memproses pembayaran: %v", err), http.StatusPaymentRequired) // 402 Payment Required
		return
	}

	// Langkah 3: Kirim Notifikasi
	if err := h.notifierSvc.SendOrderConfirmation(userID, orderData); err != nil {
		// Biasanya ini tidak menggagalkan seluruh transaksi, hanya dicatat.
		log.Printf("PERINGATAN: Gagal mengirim notifikasi untuk transaksi %s: %v", transactionID, err)
	}

	// Respon sukses
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated) // 201 Created
	json.NewEncoder(w).Encode(map[string]string{
		"message":        "Pesanan berhasil dibuat",
		"transaction_id": transactionID,
	})
}

func main() {
	// --- Wiring Dependensi NYATA di sini ---
	inventory := &LiveInventoryService{}
	payment := &LivePaymentGateway{}
	notifier := &LiveEmailNotifier{}

	orderHandler := NewOrderHandler(inventory, payment, notifier)

	mux := http.NewServeMux()
	mux.Handle("/orders", orderHandler)

	log.Println("Server berjalan di http://localhost:8080")
	http.ListenAndServe(":8080", mux)
}
