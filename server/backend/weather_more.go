package backend

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"sync"
	"time"
)

// fcCache keeps a forecast for ten minutes per (city, hours-ahead), so repeated questions do not use up a free quota.
type fcCache struct {
	mu  sync.Mutex
	m   map[string]cached
	now func() time.Time
}

func newFCCache() *fcCache { return &fcCache{m: map[string]cached{}, now: time.Now} }

func (c *fcCache) get(city string, later int) (Forecast, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	e, ok := c.m[fmt.Sprintf("%s|%d", strings.ToLower(city), later)]
	if ok && c.now().Sub(e.at) < 10*time.Minute {
		return e.f, true
	}
	return Forecast{}, false
}

func (c *fcCache) put(city string, later int, f Forecast) {
	c.mu.Lock()
	c.m[fmt.Sprintf("%s|%d", strings.ToLower(city), later)] = cached{f, c.now()}
	c.mu.Unlock()
}

func cleanCity(city string, later int) (string, int, error) {
	city = strings.TrimSpace(city)
	if city == "" || len([]rune(city)) > 80 {
		return "", 0, errors.New("weather: bad city")
	}
	if later != 0 && later != 12 {
		later = 0
	}
	return city, later, nil
}

// ---- OpenWeatherMap (free plan: current weather and the 5-day/3-hour forecast) ----

type OpenWeatherMapConfig struct {
	BaseURL   string `json:"base_url"` // default https://api.openweathermap.org
	APIKeyEnv string `json:"api_key_env"`
	TimeoutS  int    `json:"timeout_s"`
}

type OpenWeatherMap struct {
	r     *remote
	key   string
	cache *fcCache
}

func NewOpenWeatherMap(cfg OpenWeatherMapConfig) (*OpenWeatherMap, error) {
	if cfg.BaseURL == "" {
		cfg.BaseURL = "https://api.openweathermap.org"
	}
	r, err := newRemote(cfg.BaseURL, cfg.TimeoutS, 15*time.Second)
	if err != nil {
		return nil, err
	}
	return &OpenWeatherMap{r: r, key: envKey(cfg.APIKeyEnv), cache: newFCCache()}, nil
}

func owmMain(main string) string {
	switch strings.ToLower(main) {
	case "clear":
		return "sunny"
	case "rain", "drizzle", "thunderstorm":
		return "rain"
	case "snow":
		return "snow"
	case "mist", "fog", "haze":
		return "fog"
	case "dust", "sand", "ash", "smoke", "squall", "tornado":
		return "dust"
	}
	return "cloudy"
}

func (o *OpenWeatherMap) Forecast(ctx context.Context, city string, later int) (Forecast, error) {
	city, later, err := cleanCity(city, later)
	if err != nil {
		return Forecast{}, err
	}
	if f, ok := o.cache.get(city, later); ok {
		return f, nil
	}
	q := url.Values{"q": {city}, "units": {"metric"}, "appid": {o.key}}
	type cond struct {
		Weather []struct {
			Main string `json:"main"`
		} `json:"weather"`
		Main struct {
			Temp float64 `json:"temp"`
		} `json:"main"`
	}
	var c cond
	if later == 0 {
		b, _, err := o.r.call(ctx, "GET", o.r.url("/data/2.5/weather", q), nil, nil)
		if err != nil {
			return Forecast{}, err
		}
		if err := jsonUnmarshal(b, &c); err != nil || len(c.Weather) == 0 {
			return Forecast{}, errors.New("openweathermap: unexpected reply")
		}
	} else {
		q.Set("cnt", "6")
		b, _, err := o.r.call(ctx, "GET", o.r.url("/data/2.5/forecast", q), nil, nil)
		if err != nil {
			return Forecast{}, err
		}
		var rep struct {
			List []cond `json:"list"`
		}
		idx := later / 3 // 3-hour steps: +12 h is the fifth entry
		if err := jsonUnmarshal(b, &rep); err != nil || len(rep.List) <= idx || len(rep.List[idx].Weather) == 0 {
			return Forecast{}, errors.New("openweathermap: unexpected reply")
		}
		c = rep.List[idx]
	}
	f := Forecast{Main: owmMain(c.Weather[0].Main), TempC: c.Main.Temp}
	o.cache.put(city, later, f)
	return f, nil
}

// ---- WeatherAPI.com (free plan) ----

type WeatherAPIConfig struct {
	BaseURL   string `json:"base_url"` // default https://api.weatherapi.com
	APIKeyEnv string `json:"api_key_env"`
	TimeoutS  int    `json:"timeout_s"`
}

type WeatherAPI struct {
	r     *remote
	key   string
	cache *fcCache
}

func NewWeatherAPI(cfg WeatherAPIConfig) (*WeatherAPI, error) {
	if cfg.BaseURL == "" {
		cfg.BaseURL = "https://api.weatherapi.com"
	}
	r, err := newRemote(cfg.BaseURL, cfg.TimeoutS, 15*time.Second)
	if err != nil {
		return nil, err
	}
	return &WeatherAPI{r: r, key: envKey(cfg.APIKeyEnv), cache: newFCCache()}, nil
}

// weatherAPIMain maps WeatherAPI condition codes to the robot's six conditions.
func weatherAPIMain(code int) string {
	switch {
	case code == 1000:
		return "sunny"
	case code == 1003 || code == 1006 || code == 1009:
		return "cloudy"
	case code == 1030 || code == 1135 || code == 1147:
		return "fog"
	case code == 1066 || code == 1069 || code == 1114 || code == 1117 || (code >= 1204 && code <= 1237) || (code >= 1255 && code <= 1264):
		return "snow"
	case code == 1063 || code == 1072 || code == 1087 || (code >= 1150 && code <= 1201) || (code >= 1240 && code <= 1246) || (code >= 1273 && code <= 1282):
		return "rain"
	}
	return "cloudy"
}

func (w *WeatherAPI) Forecast(ctx context.Context, city string, later int) (Forecast, error) {
	city, later, err := cleanCity(city, later)
	if err != nil {
		return Forecast{}, err
	}
	if f, ok := w.cache.get(city, later); ok {
		return f, nil
	}
	b, _, err := w.r.call(ctx, "GET", w.r.url("/v1/forecast.json", url.Values{"key": {w.key}, "q": {city}, "days": {"2"}, "aqi": {"no"}, "alerts": {"no"}}), nil, nil)
	if err != nil {
		return Forecast{}, err
	}
	var rep struct {
		Location struct {
			LocalEpoch int64 `json:"localtime_epoch"`
		} `json:"location"`
		Current struct {
			TempC     float64 `json:"temp_c"`
			Condition struct {
				Code int `json:"code"`
			} `json:"condition"`
		} `json:"current"`
		Forecast struct {
			Days []struct {
				Hours []struct {
					Epoch     int64   `json:"time_epoch"`
					TempC     float64 `json:"temp_c"`
					Condition struct {
						Code int `json:"code"`
					} `json:"condition"`
				} `json:"hour"`
			} `json:"forecastday"`
		} `json:"forecast"`
	}
	if err := jsonUnmarshal(b, &rep); err != nil {
		return Forecast{}, errors.New("weatherapi: unexpected reply")
	}
	f := Forecast{Main: weatherAPIMain(rep.Current.Condition.Code), TempC: rep.Current.TempC}
	if later == 12 {
		target := rep.Location.LocalEpoch + 12*3600
		best, bestD := -1, int64(1<<62)
		var hours []struct {
			Epoch int64
			T     float64
			Code  int
		}
		for _, d := range rep.Forecast.Days {
			for _, h := range d.Hours {
				hours = append(hours, struct {
					Epoch int64
					T     float64
					Code  int
				}{h.Epoch, h.TempC, h.Condition.Code})
			}
		}
		for i, h := range hours {
			if d := abs64(h.Epoch - target); d < bestD {
				best, bestD = i, d
			}
		}
		if best < 0 || bestD > 3*3600 {
			return Forecast{}, errors.New("weatherapi: no forecast hour")
		}
		f = Forecast{Main: weatherAPIMain(hours[best].Code), TempC: hours[best].T}
	}
	w.cache.put(city, later, f)
	return f, nil
}

func abs64(v int64) int64 {
	if v < 0 {
		return -v
	}
	return v
}
