package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"math/rand"
	"net/http"
)

// --- Tipe Data ---
type Transaction struct {
	ID        int
	ProductID string
	Region    string
	Amount    float64
}

type RegionalSummary struct {
	TotalSales float64 `json:"total_sales"`
	NumTrans   int     `json:"number_of_transactions"`
}

type SalesReport struct {
	RegionalSales map[string]RegionalSummary `json:"regional_sales"`
}

// --- Interfaces untuk Dependensi ---
type TransactionFetcher interface {
	FetchTransactions(recordCount int) ([]Transaction, error)
}

type ReportAggregator interface {
	AggregateByRegion(transactions []Transaction) SalesReport
}

// --- Implementasi NYATA (untuk produksi) ---
type LiveTransactionFetcher struct{}

// Fungsi inilah yang menjadi sumber utama penggunaan memori.
func (f *LiveTransactionFetcher) FetchTransactions(recordCount int) ([]Transaction, error) {
	log.Printf("NYATA: Mengalokasikan memori untuk %d data transaksi...", recordCount)
	if recordCount > 100000 {
		return nil, errors.New("permintaan data terlalu besar")
	}

	// Alokasikan slice besar di memori.
	transactions := make([]Transaction, recordCount)
	regions := []string{"Asia", "Europe", "North America", "South America", "Africa"}

	// Isi slice dengan data dummy.
	for i := 0; i < recordCount; i++ {
		transactions[i] = Transaction{
			ID:        i,
			ProductID: fmt.Sprintf("PROD-%d", rand.Intn(1000)),
			Region:    regions[rand.Intn(len(regions))],
			Amount:    rand.Float64() * 500,
		}
	}
	log.Printf("NYATA: Selesai membuat %d data transaksi di memori.", recordCount)
	return transactions, nil
}

type LiveReportAggregator struct{}

// Fungsi ini juga menggunakan memori untuk membuat map agregasi.
func (a *LiveReportAggregator) AggregateByRegion(transactions []Transaction) SalesReport {
	log.Println("NYATA: Memulai agregasi data...")
	summary := make(map[string]RegionalSummary)

	for _, tx := range transactions {
		regionData := summary[tx.Region]
		regionData.TotalSales += tx.Amount
		regionData.NumTrans++
		summary[tx.Region] = regionData
	}

	log.Println("NYATA: Selesai melakukan agregasi.")
	return SalesReport{RegionalSales: summary}
}

// --- Handler Utama ---
type ReportHandler struct {
	fetcher    TransactionFetcher
	aggregator ReportAggregator
}

func NewReportHandler(f TransactionFetcher, a ReportAggregator) *ReportHandler {
	return &ReportHandler{f, a}
}

func (h *ReportHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	var reqData struct {
		RecordCount int `json:"record_count"`
	}
	if err := json.NewDecoder(r.Body).Decode(&reqData); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	// 1. Fetch (Memory Allocation)
	transactions, err := h.fetcher.FetchTransactions(reqData.RecordCount)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// 2. Aggregate (Memory Processing)
	report := h.aggregator.AggregateByRegion(transactions)

	// 3. Respond
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(report)
}

func main() {
	handler := NewReportHandler(
		&LiveTransactionFetcher{},
		&LiveReportAggregator{},
	)
	mux := http.NewServeMux()
	mux.Handle("/generate-sales-report", handler)

	log.Println("Server pelaporan penjualan berjalan di http://localhost:8080")
	http.ListenAndServe(":8080", mux)
}
