package greeks

import "math"

// NormalCDF calculates the cumulative distribution function N(x)
// This is the probability that a standard normal random variable is <= x
// Uses Abramowitz and Stegun approximation with accuracy ~7.5e-8
func NormalCDF(x float64) float64 {
	// Handle extreme values
	if x < -10 {
		return 0.0
	}
	if x > 10 {
		return 1.0
	}

	// Abramowitz and Stegun approximation constants
	const (
		a1 = 0.254829592
		a2 = -0.284496736
		a3 = 1.421413741
		a4 = -1.453152027
		a5 = 1.061405429
		p  = 0.3275911
	)

	sign := 1.0
	if x < 0 {
		sign = -1.0
	}
	x = math.Abs(x) / math.Sqrt2

	t := 1.0 / (1.0 + p*x)
	y := 1.0 - (((((a5*t+a4)*t)+a3)*t+a2)*t+a1)*t*math.Exp(-x*x)

	return 0.5 * (1.0 + sign*y)
}

// NormalPDF calculates the probability density function N'(x)
// This is used for Gamma and Vega calculations
func NormalPDF(x float64) float64 {
	return math.Exp(-0.5*x*x) / math.Sqrt(2*math.Pi)
}
