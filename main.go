package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

// --- Struct untuk menampung semua data ---
type WeatherData struct {
	Temperature   float64 `json:"temperature"`
	Precipitation float64 `json:"precipitation"`
	CloudCover    int     `json:"cloud_cover"`
	UVIndex       float64 `json:"uv_index"`
	AQI           int     `json:"aqi"`
}

type SunData struct {
	Sunrise    string `json:"sunrise"`
	Sunset     string `json:"sunset"`
	GoldenHour string `json:"golden_hour_end"`
}

type MoonData struct {
	PhaseName    string  `json:"phase_name"`
	Illumination float64 `json:"illumination"`
}

type CalculatedIndices struct {
	HikingIndex          float64 `json:"hiking_index"`
	HikingRecommendation string  `json:"hiking_recommendation"`
}

// Struct final yang akan dikirim sebagai JSON ke Flutter
type ConsolidatedResponse struct {
	Weather WeatherData       `json:"weather"`
	Sun     SunData           `json:"sun"`
	Moon    MoonData          `json:"moon"`
	Indices CalculatedIndices `json:"indices"`
}

func main() {
	r := gin.Default()
	// Endpoint utama aplikasi
	r.GET("/weather/:lat/:lon", getConsolidatedData)
	fmt.Println("Backend server berjalan di http://localhost:8080")
	r.Run(":8080")
}

func getConsolidatedData(c *gin.Context) {
	lat := c.Param("lat")
	lon := c.Param("lon")

	var weather WeatherData
	var sun SunData
	var wg sync.WaitGroup // Untuk menjalankan API call secara bersamaan

	// 1. Ambil data cuaca dan AQI dari Open-Meteo
	wg.Add(1)
	go func() {
		defer wg.Done()
		weather, _ = fetchWeatherData(lat, lon)
	}()

	// 2. Ambil data matahari dari Sunrise-Sunset.org
	wg.Add(1)
	go func() {
		defer wg.Done()
		sun, _ = fetchSunData(lat, lon)
	}()

	wg.Wait() // Tunggu semua API call selesai

	// 3. Kalkulasi data bulan (lokal)
	moon := calculateMoonPhase()

	// 4. Kalkulasi Indeks (lokal)
	indices := calculateIndices(weather)

	// Gabungkan semua data menjadi satu
	response := ConsolidatedResponse{
		Weather: weather,
		Sun:     sun,
		Moon:    moon,
		Indices: indices,
	}

	c.JSON(http.StatusOK, response)
}

// --- Fungsi Helper ---

func fetchWeatherData(lat, lon string) (WeatherData, error) {
	url := fmt.Sprintf("https://api.open-meteo.com/v1/forecast?latitude=%s&longitude=%s&current=temperature_2m,precipitation,cloud_cover,uv_index&hourly=european_aqi&timezone=auto&forecast_days=1", lat, lon)

	resp, err := http.Get(url)
	if err != nil {
		return WeatherData{}, err
	}
	defer resp.Body.Close()

	var result struct {
		Current struct {
			Temperature   float64 `json:"temperature_2m"`
			Precipitation float64 `json:"precipitation"`
			CloudCover    int     `json:"cloud_cover"`
			UVIndex       float64 `json:"uv_index"`
		} `json:"current"`
		Hourly struct {
			AQI []int `json:"european_aqi"`
		} `json:"hourly"`
	}

	json.NewDecoder(resp.Body).Decode(&result)

	aqi := 0
	if len(result.Hourly.AQI) > time.Now().Hour() {
		aqi = result.Hourly.AQI[time.Now().Hour()]
	}

	return WeatherData{
		Temperature:   result.Current.Temperature,
		Precipitation: result.Current.Precipitation,
		CloudCover:    result.Current.CloudCover,
		UVIndex:       result.Current.UVIndex,
		AQI:           aqi,
	}, nil
}

func fetchSunData(lat, lon string) (SunData, error) {
	url := fmt.Sprintf("https://api.sunrise-sunset.org/json?lat=%s&lng=%s&formatted=0", lat, lon)
	resp, err := http.Get(url)
	if err != nil {
		return SunData{}, err
	}
	defer resp.Body.Close()

	var result struct {
		Results struct {
			Sunrise    string `json:"sunrise"`
			Sunset     string `json:"sunset"`
			GoldenHour string `json:"golden_hour"`
		} `json:"results"`
	}
	json.NewDecoder(resp.Body).Decode(&result)

	// Konversi waktu UTC ke Waktu Lokal (WIB)
	sunriseUTC, _ := time.Parse(time.RFC3339, result.Results.Sunrise)
	sunsetUTC, _ := time.Parse(time.RFC3339, result.Results.Sunset)
	goldenHourUTC, _ := time.Parse(time.RFC3339, result.Results.GoldenHour)

	loc, _ := time.LoadLocation("Asia/Jakarta")

	return SunData{
		Sunrise:    sunriseUTC.In(loc).Format("15:04"),
		Sunset:     sunsetUTC.In(loc).Format("15:04"),
		GoldenHour: goldenHourUTC.In(loc).Format("15:04"),
	}, nil
}

func calculateMoonPhase() MoonData {
	// Implementasi sederhana, di dunia nyata bisa menggunakan library astronomi
	return MoonData{PhaseName: "Sabit Awal", Illumination: 0.25}
}

func calculateIndices(weather WeatherData) CalculatedIndices {
	score := 10.0
	if weather.Temperature > 32 || weather.Temperature < 15 {
		score -= 2
	}
	if weather.Precipitation > 0.5 {
		score -= 3
	}
	if weather.UVIndex > 7 {
		score -= 1.5
	}
	if weather.AQI > 100 {
		score -= 1.5
	}
	if score < 1 {
		score = 1.0
	}

	var recommendation string
	if score >= 8 {
		recommendation = "Sangat baik untuk mendaki!"
	} else if score >= 5 {
		recommendation = "Cukup baik, tetap waspada."
	} else {
		recommendation = "Kurang ideal, pertimbangkan kembali."
	}

	return CalculatedIndices{
		HikingIndex:          score,
		HikingRecommendation: recommendation,
	}
}
