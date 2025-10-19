package main

import (
	"encoding/json"
	"net/http"
)

// RequestBody defines the structure for the incoming JSON request
type RequestBody struct {
	Price        float64 `json:"price"`
	UserCategory string  `json:"user_category"`
}

// ResponseBody defines the structure for the JSON response
type ResponseBody struct {
	OriginalPrice float64 `json:"original_price"`
	Discount      float64 `json:"discount"`
	FinalPrice    float64 `json:"final_price"`
}

// calculateDiscount is our pure business logic function.
// It's easy to test this function on its own (unit test).
func calculateDiscount(price float64, category string) (float64, float64) {
	var discountRate float64
	switch category {
	case "premium":
		discountRate = 0.20 // 20% discount
	case "gold":
		discountRate = 0.10 // 10% discount
	default:
		discountRate = 0.0 // 0% discount
	}

	discountAmount := price * discountRate
	finalPrice := price - discountAmount
	return discountAmount, finalPrice
}

// discountHandler is the HTTP handler we want to test.
func discountHandler(w http.ResponseWriter, r *http.Request) {
	var reqBody RequestBody

	// Decode the request body
	err := json.NewDecoder(r.Body).Decode(&reqBody)
	if err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	// Validate input
	if reqBody.Price <= 0 {
		http.Error(w, "Price must be positive", http.StatusBadRequest)
		return
	}

	// Use the business logic
	discount, finalPrice := calculateDiscount(reqBody.Price, reqBody.UserCategory)

	// Prepare the response
	resBody := ResponseBody{
		OriginalPrice: reqBody.Price,
		Discount:      discount,
		FinalPrice:    finalPrice,
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(resBody)
}

// pingHandler is a simple handler for health checks.
func pingHandler(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("pong"))
}

func main() {
	http.HandleFunc("/ping", pingHandler)
	http.HandleFunc("/calculate-discount", discountHandler)
	// For testing, we don't need to start the server.
	// But to run it for k6, we must start it.
	http.ListenAndServe(":8080", nil)
}
