package main

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"sync"
	"time"
)

// --- Tipe Data untuk Hasil ---
type FlightDeal struct {
	Airline string  `json:"airline"`
	Price   float64 `json:"price"`
}

type HotelDeal struct {
	HotelName string  `json:"hotel_name"`
	Price     float64 `json:"price_per_night"`
}

type ActivityDeal struct {
	Name        string  `json:"name"`
	Price       float64 `json:"price_per_person"`
	Description string  `json:"description"`
}

// --- Interfaces untuk Dependensi Eksternal ---
type FlightFinder interface {
	FindFlights(destination string) ([]FlightDeal, error)
}
type HotelFinder interface {
	FindHotels(destination string) ([]HotelDeal, error)
}
type ActivityFinder interface {
	FindActivities(destination string) ([]ActivityDeal, error)
}

// --- Implementasi NYATA (untuk produksi) ---
type LiveFlightFinder struct{}

func (f *LiveFlightFinder) FindFlights(destination string) ([]FlightDeal, error) {
	log.Println("NYATA: Mencari penerbangan...")
	time.Sleep(150 * time.Millisecond) // Simulasikan latensi jaringan
	if destination == "Tokyo" {
		return []FlightDeal{{Airline: "JAL", Price: 1200.50}, {Airline: "ANA", Price: 1250.00}}, nil
	}
	if destination == "Bali" {
		return nil, errors.New("tidak ada penerbangan tersedia untuk Bali")
	}
	return []FlightDeal{{Airline: "Generic Air", Price: 800.00}}, nil
}

type LiveHotelFinder struct{}

func (f *LiveHotelFinder) FindHotels(destination string) ([]HotelDeal, error) {
	log.Println("NYATA: Mencari hotel...")
	time.Sleep(200 * time.Millisecond) // Simulasikan latensi yang sedikit lebih lama
	if destination == "Paris" {
		return []HotelDeal{{HotelName: "Ritz", Price: 990.00}}, nil
	}
	return []HotelDeal{{HotelName: "Grand Hyatt", Price: 350.75}}, nil
}

type LiveActivityFinder struct{}

func (f *LiveActivityFinder) FindActivities(destination string) ([]ActivityDeal, error) {
	log.Println("NYATA: Mencari aktivitas...")
	time.Sleep(100 * time.Millisecond)
	if destination == "Paris" {
		return nil, errors.New("layanan aktivitas sedang down")
	}
	return []ActivityDeal{{Name: "City Tour", Price: 75.00, Description: "Jelajahi kota"}}, nil
}

// --- Handler Utama yang Mengatur Alur Kerja ---
type ItineraryHandler struct {
	flightSvc   FlightFinder
	hotelSvc    HotelFinder
	activitySvc ActivityFinder
}

func NewItineraryHandler(f FlightFinder, h HotelFinder, a ActivityFinder) *ItineraryHandler {
	return &ItineraryHandler{f, h, a}
}

type ItineraryResponse struct {
	Flights    []FlightDeal   `json:"flights,omitempty"`
	Hotels     []HotelDeal    `json:"hotels,omitempty"`
	Activities []ActivityDeal `json:"activities,omitempty"`
	Errors     []string       `json:"errors,omitempty"`
}

func (h *ItineraryHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	var reqData struct {
		Destination string `json:"destination"`
	}
	if err := json.NewDecoder(r.Body).Decode(&reqData); err != nil {
		http.Error(w, "Invalid request", http.StatusBadRequest)
		return
	}

	response := ItineraryResponse{}
	var wg sync.WaitGroup
	var mu sync.Mutex // Untuk melindungi akses ke 'response'

	// 1. Jalankan semua pencarian secara paralel
	wg.Add(3)

	go func() {
		defer wg.Done()
		flights, err := h.flightSvc.FindFlights(reqData.Destination)
		mu.Lock()
		defer mu.Unlock()
		if err != nil {
			response.Errors = append(response.Errors, fmt.Sprintf("Penerbangan: %v", err))
		} else {
			response.Flights = flights
		}
	}()

	go func() {
		defer wg.Done()
		hotels, err := h.hotelSvc.FindHotels(reqData.Destination)
		mu.Lock()
		defer mu.Unlock()
		if err != nil {
			response.Errors = append(response.Errors, fmt.Sprintf("Hotel: %v", err))
		} else {
			response.Hotels = hotels
		}
	}()

	go func() {
		defer wg.Done()
		activities, err := h.activitySvc.FindActivities(reqData.Destination)
		mu.Lock()
		defer mu.Unlock()
		if err != nil {
			response.Errors = append(response.Errors, fmt.Sprintf("Aktivitas: %v", err))
		} else {
			response.Activities = activities
		}
	}()

	// 2. Tunggu semua pencarian selesai
	wg.Wait()

	// 3. Kirim respons yang sudah diagregasi
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

func main() {
	handler := NewItineraryHandler(
		&LiveFlightFinder{},
		&LiveHotelFinder{},
		&LiveActivityFinder{},
	)
	mux := http.NewServeMux()
	mux.Handle("/generate-itinerary", handler)

	log.Println("Server agregator perjalanan berjalan di http://localhost:8080")
	http.ListenAndServe(":8080", mux)
}

