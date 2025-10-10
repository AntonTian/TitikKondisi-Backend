package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"
	"math"
	"github.com/gin-gonic/gin"
)

// --- Struct untuk data cuaca, matahari, bulan, dan indeks ---
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

type ConsolidatedResponse struct {
	Weather WeatherData       `json:"weather"`
	Sun     SunData           `json:"sun"`
	Moon    MoonData          `json:"moon"`
	Indices CalculatedIndices `json:"indices"`
}

func main() {
	r := gin.Default()

	// --- Dua endpoint: GET dan POST ---
	r.GET("/weather/:lat/:lon", getWeatherByParams)
	r.POST("/weather", getWeatherByJSON)

	fmt.Println("Server berjalan di http://localhost:8080")
	r.Run(":8080")
}

// --- Handler untuk GET (pakai URL params) ---
func getWeatherByParams(c *gin.Context) {
	lat := c.Param("lat")
	lon := c.Param("lon")
	response, err := getConsolidatedData(lat, lon)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, response)
}

// --- Handler untuk POST (pakai JSON body) ---
func getWeatherByJSON(c *gin.Context) {
	var input struct {
		Lat string `json:"lat"`
		Lon string `json:"lon"`
	}
	if err := c.BindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request body"})
		return
	}

	response, err := getConsolidatedData(input.Lat, input.Lon)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, response)
}

// --- Fungsi utama untuk ambil semua data ---
func getConsolidatedData(lat, lon string) (ConsolidatedResponse, error) {
	var weather WeatherData
	var sun SunData
	var wg sync.WaitGroup
	var err1, err2 error

	wg.Add(1)
	go func() {
		defer wg.Done()
		weather, err1 = fetchWeatherData(lat, lon)
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		sun, err2 = fetchSunData(lat, lon)
	}()

	wg.Wait()

	if err1 != nil {
		return ConsolidatedResponse{}, err1
	}
	if err2 != nil {
		return ConsolidatedResponse{}, err2
	}

	moon := calculateMoonPhase()
	indices := calculateIndices(weather)

	return ConsolidatedResponse{
		Weather: weather,
		Sun:     sun,
		Moon:    moon,
		Indices: indices,
	}, nil
}

// --- API Call ke Open-Meteo ---
func fetchWeatherData(lat, lon string) (WeatherData, error) {
	// --- Fetch main weather ---
	weatherURL := fmt.Sprintf(
		"https://api.open-meteo.com/v1/forecast?latitude=%s&longitude=%s&current=temperature_2m,precipitation,cloud_cover,uv_index&timezone=auto",
		lat, lon,
	)

	resp1, err := http.Get(weatherURL)
	if err != nil {
		return WeatherData{}, fmt.Errorf("weather fetch error: %v", err)
	}
	defer resp1.Body.Close()

	if resp1.StatusCode != 200 {
		return WeatherData{}, fmt.Errorf("weather bad response: %s", resp1.Status)
	}

	var weatherResult struct {
		Current struct {
			Temperature   float64 `json:"temperature_2m"`
			Precipitation float64 `json:"precipitation"`
			CloudCover    int     `json:"cloud_cover"`
			UVIndex       float64 `json:"uv_index"`
		} `json:"current"`
	}

	if err := json.NewDecoder(resp1.Body).Decode(&weatherResult); err != nil {
		return WeatherData{}, fmt.Errorf("weather JSON decode error: %v", err)
	}

	// --- Fetch AQI separately ---
	aqiURL := fmt.Sprintf(
		"https://air-quality-api.open-meteo.com/v1/air-quality?latitude=%s&longitude=%s&hourly=european_aqi&timezone=auto",
		lat, lon,
	)
	resp2, err := http.Get(aqiURL)
	if err != nil {
		return WeatherData{}, fmt.Errorf("aqi fetch error: %v", err)
	}
	defer resp2.Body.Close()

	var aqiResult struct {
		Hourly struct {
			AQI []int `json:"european_aqi"`
		} `json:"hourly"`
	}

	if err := json.NewDecoder(resp2.Body).Decode(&aqiResult); err != nil {
		fmt.Println("AQI decode error:", err)
	}

	aqi := 0
	if len(aqiResult.Hourly.AQI) > 0 {
		// Use latest value (last in slice)
		aqi = aqiResult.Hourly.AQI[len(aqiResult.Hourly.AQI)-1]
	}

	return WeatherData{
		Temperature:   weatherResult.Current.Temperature,
		Precipitation: weatherResult.Current.Precipitation,
		CloudCover:    weatherResult.Current.CloudCover,
		UVIndex:       weatherResult.Current.UVIndex,
		AQI:           aqi,
	}, nil
}

// --- API Call ke Sunrise-Sunset (fix golden hour) ---
func fetchSunData(lat, lon string) (SunData, error) {
	url := fmt.Sprintf("https://api.sunrise-sunset.org/json?lat=%s&lng=%s&formatted=0", lat, lon)
	resp, err := http.Get(url)
	if err != nil {
		return SunData{}, err
	}
	defer resp.Body.Close()

	var result struct {
		Results struct {
			Sunrise string `json:"sunrise"`
			Sunset  string `json:"sunset"`
		} `json:"results"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return SunData{}, err
	}

	sunriseUTC, err1 := time.Parse(time.RFC3339, result.Results.Sunrise)
	sunsetUTC, err2 := time.Parse(time.RFC3339, result.Results.Sunset)
	if err1 != nil || err2 != nil {
		return SunData{}, fmt.Errorf("invalid time format")
	}

	loc, _ := time.LoadLocation("Asia/Jakarta")
	sunriseLocal := sunriseUTC.In(loc)
	sunsetLocal := sunsetUTC.In(loc)
	goldenHourEnd := sunriseLocal.Add(time.Hour)

	return SunData{
		Sunrise:    sunriseLocal.Format("15:04"),
		Sunset:     sunsetLocal.Format("15:04"),
		GoldenHour: goldenHourEnd.Format("15:04"),
	}, nil
}

// --- Calculate Moon Phase ---
func calculateMoonPhase() MoonData {
	now := time.Now().UTC()
	newMoon := time.Date(2000, 1, 6, 18, 14, 0, 0, time.UTC)
	days := now.Sub(newMoon).Hours() / 24
	phase := math.Mod(days, 29.53058867) / 29.53058867

	var phaseName string
	switch {
	case phase < 0.03 || phase > 0.97:
		phaseName = "Bulan Baru"
	case phase < 0.25:
		phaseName = "Sabit Awal"
	case phase < 0.27:
		phaseName = "Kuartal Pertama"
	case phase < 0.50:
		phaseName = "Cembung Awal"
	case phase < 0.53:
		phaseName = "Bulan Purnama"
	case phase < 0.75:
		phaseName = "Cembung Akhir"
	case phase < 0.77:
		phaseName = "Kuartal Akhir"
	default:
		phaseName = "Sabit Akhir"
	}

	illum := phase
	if illum > 0.5 {
		illum = 1 - illum
	}
	illum *= 2

	return MoonData{PhaseName: phaseName, Illumination: math.Round(illum*100) / 100}
}

// --- Calculate Hiking Index ---
func calculateIndices(weather WeatherData) CalculatedIndices {
	score := 10

	if weather.Temperature > 33 {
		score -= 3
	} else if weather.Temperature < 18 {
		score -= 2
	}

	if weather.Precipitation > 1 {
		score -= 4
	}

	if weather.UVIndex > 8 {
		score -= 2
	}

	if weather.AQI > 100 {
		score -= 3
	}

	if weather.CloudCover > 80 {
		score -= 1
	}

	if score < 0 {
		score = 0
	} else if score > 10 {
		score = 10
	}

	var recommendation string
	switch {
	case score >= 8:
		recommendation = "Sangat baik untuk mendaki!"
	case score >= 5:
		recommendation = "Cukup baik, tetapi perhatikan cuaca."
	case score >= 3:
		recommendation = "Kurang disarankan, kondisi tidak ideal."
	default:
		recommendation = "Tidak disarankan untuk mendaki hari ini."
	}

	return CalculatedIndices{
		HikingIndex:          math.Round(float64(score) * 10) / 10,
		HikingRecommendation: recommendation,
	}
}
