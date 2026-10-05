package temperature

import (
	"encoding/json"
	"testing"
)

func TestCelsiusToFahrenheit(t *testing.T) {
	tests := []struct {
		celsius, want float64
	}{
		{0, 32},
		{100, 212},
		{-40, -40},
		{37, 98.6},
	}
	for _, tt := range tests {
		if got := round(CelsiusToFahrenheit(tt.celsius)); got != tt.want {
			t.Errorf("CelsiusToFahrenheit(%v) = %v, want %v", tt.celsius, got, tt.want)
		}
	}
}

func TestCelsiusToKelvin(t *testing.T) {
	tests := []struct {
		celsius, want float64
	}{
		{0, 273},
		{100, 373},
		{-273, 0},
		{28.5, 301.5},
	}
	for _, tt := range tests {
		if got := round(CelsiusToKelvin(tt.celsius)); got != tt.want {
			t.Errorf("CelsiusToKelvin(%v) = %v, want %v", tt.celsius, got, tt.want)
		}
	}
}

func TestFromCelsius(t *testing.T) {
	got := FromCelsius(28.5)
	want := Temperatures{Celsius: 28.5, Fahrenheit: 83.3, Kelvin: 301.5}
	if got != want {
		t.Fatalf("FromCelsius(28.5) = %+v, want %+v", got, want)
	}
}

func TestTemperaturesJSON(t *testing.T) {
	b, err := json.Marshal(FromCelsius(28.5))
	if err != nil {
		t.Fatal(err)
	}
	want := `{"temp_C":28.5,"temp_F":83.3,"temp_K":301.5}`
	if string(b) != want {
		t.Fatalf("json = %s, want %s", b, want)
	}
}
