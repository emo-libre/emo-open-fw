package backend

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

// StaticWeather always answers the same thing (demo / offline use).
type StaticWeather struct{ F Forecast }

func (s StaticWeather) Forecast(ctx context.Context, city string, later int) (Forecast, error) {
	return s.F, nil
}

// OpenMeteoConfig points at an Open-Meteo-compatible API. It is opt-in: the city name is sent to that host.
type OpenMeteoConfig struct {
	GeocodeURL  string `json:"geocode_url"`  // default https://geocoding-api.open-meteo.com
	ForecastURL string `json:"forecast_url"` // default https://api.open-meteo.com
}

type OpenMeteo struct {
	gc, fc   *http.Client
	gcu, fcu *url.URL
	mu       sync.Mutex
	cache    map[string]cached
	now      func() time.Time
}

type cached struct {
	f  Forecast
	at time.Time
}

func NewOpenMeteo(cfg OpenMeteoConfig) (*OpenMeteo, error) {
	if cfg.GeocodeURL == "" {
		cfg.GeocodeURL = "https://geocoding-api.open-meteo.com"
	}
	if cfg.ForecastURL == "" {
		cfg.ForecastURL = "https://api.open-meteo.com"
	}
	gc, gcu, err := NewGuardedClient(cfg.GeocodeURL, 15*time.Second)
	if err != nil {
		return nil, err
	}
	fc, fcu, err := NewGuardedClient(cfg.ForecastURL, 15*time.Second)
	if err != nil {
		return nil, err
	}
	return &OpenMeteo{gc: gc, fc: fc, gcu: gcu, fcu: fcu, cache: map[string]cached{}, now: time.Now}, nil
}

// wmoToMain maps WMO weather codes to the six conditions the robot knows.
func wmoToMain(code int) string {
	switch {
	case code == 0 || code == 1:
		return "sunny"
	case code == 2 || code == 3:
		return "cloudy"
	case code == 45 || code == 48:
		return "fog"
	case (code >= 71 && code <= 77) || code == 85 || code == 86:
		return "snow"
	case (code >= 51 && code <= 67) || (code >= 80 && code <= 82) || code >= 95:
		return "rain"
	}
	return "cloudy"
}

func getJSON(ctx context.Context, c *http.Client, u string, v any) error {
	req, err := http.NewRequestWithContext(ctx, "GET", u, nil)
	if err != nil {
		return err
	}
	resp, err := c.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return fmt.Errorf("weather: HTTP %d", resp.StatusCode)
	}
	return json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(v)
}

func (o *OpenMeteo) Forecast(ctx context.Context, city string, later int) (Forecast, error) {
	city = strings.TrimSpace(city)
	if city == "" || len(city) > 80 {
		return Forecast{}, errors.New("weather: bad city")
	}
	if later != 0 && later != 12 {
		later = 0
	}
	key := fmt.Sprintf("%s|%d", strings.ToLower(city), later)
	o.mu.Lock()
	if c, ok := o.cache[key]; ok && o.now().Sub(c.at) < 10*time.Minute {
		o.mu.Unlock()
		return c.f, nil
	}
	o.mu.Unlock()

	var geo struct {
		Results []struct {
			Latitude  float64 `json:"latitude"`
			Longitude float64 `json:"longitude"`
		} `json:"results"`
	}
	gu := *o.gcu
	gu.Path = "/v1/search"
	gu.RawQuery = url.Values{"name": {city}, "count": {"1"}}.Encode()
	if err := getJSON(ctx, o.gc, gu.String(), &geo); err != nil {
		return Forecast{}, err
	}
	if len(geo.Results) == 0 {
		return Forecast{}, errors.New("weather: city not found")
	}
	var fc struct {
		Hourly struct {
			Temp []float64 `json:"temperature_2m"`
			Code []int     `json:"weather_code"`
		} `json:"hourly"`
	}
	fu := *o.fcu
	fu.Path = "/v1/forecast"
	fu.RawQuery = url.Values{
		"latitude":       {fmt.Sprintf("%.4f", geo.Results[0].Latitude)},
		"longitude":      {fmt.Sprintf("%.4f", geo.Results[0].Longitude)},
		"hourly":         {"temperature_2m,weather_code"},
		"forecast_hours": {"14"},
		"timezone":       {"auto"},
	}.Encode()
	if err := getJSON(ctx, o.fc, fu.String(), &fc); err != nil {
		return Forecast{}, err
	}
	if len(fc.Hourly.Temp) <= later || len(fc.Hourly.Code) <= later {
		return Forecast{}, errors.New("weather: short forecast")
	}
	f := Forecast{Main: wmoToMain(fc.Hourly.Code[later]), TempC: fc.Hourly.Temp[later]}
	o.mu.Lock()
	o.cache[key] = cached{f, o.now()}
	o.mu.Unlock()
	return f, nil
}
