// Package temperature converts temperatures between Celsius, Fahrenheit and Kelvin.
package temperature

import "math"

// Temperatures holds the same temperature expressed in the three supported scales.
type Temperatures struct {
	Celsius    float64 `json:"temp_C"`
	Fahrenheit float64 `json:"temp_F"`
	Kelvin     float64 `json:"temp_K"`
}

// CelsiusToFahrenheit applies F = C × 1.8 + 32.
func CelsiusToFahrenheit(c float64) float64 {
	return c*1.8 + 32
}

// CelsiusToKelvin applies K = C + 273, as defined by the challenge.
func CelsiusToKelvin(c float64) float64 {
	return c + 273
}

// FromCelsius builds Temperatures from a Celsius value, rounding every scale to
// two decimal places to hide floating point noise (e.g. 83.30000000000001).
func FromCelsius(c float64) Temperatures {
	return Temperatures{
		Celsius:    round(c),
		Fahrenheit: round(CelsiusToFahrenheit(c)),
		Kelvin:     round(CelsiusToKelvin(c)),
	}
}

func round(v float64) float64 {
	return math.Round(v*100) / 100
}
