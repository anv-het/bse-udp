package greeks

import (
	"math"
	"time"
)

// AdvancedGreeks contains second-order and cross Greeks
type AdvancedGreeks struct {
	Vanna float64 // ∂²V/∂S∂σ - Sensitivity of Delta to volatility
	Vomma float64 // ∂²V/∂σ² - Sensitivity of Vega to volatility
	Charm float64 // ∂²V/∂S∂t - Delta decay over time (per day)
}

// CalculateAdvanced computes second-order and cross Greeks
func (c *Calculator) CalculateAdvanced(
	optionType string,
	spotPrice, strikePrice float64,
	expiryDate time.Time,
	sigma float64,
) AdvancedGreeks {
	timeToExpiry := time.Until(expiryDate).Hours() / (24 * 365.25)

	if timeToExpiry <= 0 || sigma <= 0 {
		return AdvancedGreeks{
			Vanna: 0,
			Vomma: 0,
			Charm: 0,
		}
	}

	// Calculate basic Greeks first (needed for advanced Greeks)
	basicGreeks := c.Calculate(optionType, spotPrice, strikePrice, expiryDate, sigma)

	// Calculate d1 and d2 (needed for formulas)
	sqrtT := math.Sqrt(timeToExpiry)
	d1 := (math.Log(spotPrice/strikePrice) +
		(c.riskFreeRate+0.5*sigma*sigma)*timeToExpiry) / (sigma * sqrtT)
	d2 := d1 - sigma*sqrtT

	// Calculate advanced Greeks
	vanna := c.calculateVanna(basicGreeks.Vega, spotPrice, d1, sigma, sqrtT)
	vomma := c.calculateVomma(basicGreeks.Vega, d1, d2, sigma)
	charm := c.calculateCharm(optionType, spotPrice, strikePrice, d1, d2,
		timeToExpiry, sigma)

	return AdvancedGreeks{
		Vanna: vanna,
		Vomma: vomma,
		Charm: charm,
	}
}

// calculateVanna computes Vanna (cross-gamma)
// Vanna = ∂²V/∂S∂σ = (Vega/S) × (1 - d1/(σ√t))
// Measures: How Delta changes with volatility
func (c *Calculator) calculateVanna(
	vega, spotPrice, d1, sigma, sqrtT float64,
) float64 {
	if spotPrice <= 0 || sigma <= 0 || sqrtT <= 0 {
		return 0
	}

	// Vanna = (Vega/S) × (1 - d1/(σ√t))
	// Vega is per 1% change, so adjust to per-unit change for calculation
	vegaUnit := vega * 100.0 // Convert from per 1% to per unit (0.01)
	vanna := (vegaUnit / spotPrice) * (1 - d1/(sigma*sqrtT))

	return vanna
}

// calculateVomma computes Vomma (volatility gamma)
// Vomma = ∂²V/∂σ² = Vega × (d1 × d2 / σ)
// Measures: How Vega changes with volatility (convexity in vol)
func (c *Calculator) calculateVomma(
	vega, d1, d2, sigma float64,
) float64 {
	if sigma <= 0 {
		return 0
	}

	// Vomma = Vega × (d1 × d2 / σ)
	// Vega is per 1% change, so result is also per 1% change
	vomma := vega * (d1 * d2 / sigma)

	return vomma
}

// calculateCharm computes Charm (Delta decay)
// Charm = -∂²V/∂S∂t = -∂Delta/∂t
// Measures: How Delta changes as time passes (per day)
//
// For Call: Charm = -N'(d1) × [2rt - d2×σ] / [2t×σ√t] - r×K×e^(-rt)×N(d2)
// For Put:  Charm = -N'(d1) × [2rt - d2×σ] / [2t×σ√t] + r×K×e^(-rt)×N(-d2)
func (c *Calculator) calculateCharm(
	optionType string,
	spotPrice, strikePrice, d1, d2, timeToExpiry, sigma float64,
) float64 {
	if timeToExpiry <= 0 || sigma <= 0 {
		return 0
	}

	sqrtT := math.Sqrt(timeToExpiry)

	// Calculate N'(d1) - Normal PDF at d1
	nd1 := NormalPDF(d1)

	// Common term: -N'(d1) × [2rt - d2×σ] / [2t×σ√t]
	term1 := -nd1 * (2*c.riskFreeRate*timeToExpiry - d2*sigma) /
		(2 * timeToExpiry * sigma * sqrtT)

	// Discount factor
	discountFactor := math.Exp(-c.riskFreeRate * timeToExpiry)

	var charm float64

	if optionType == "CE" {
		// Call Charm
		term2 := c.riskFreeRate * strikePrice * discountFactor * NormalCDF(d2)
		charm = term1 - term2
	} else {
		// Put Charm
		term2 := c.riskFreeRate * strikePrice * discountFactor * NormalCDF(-d2)
		charm = term1 + term2
	}

	// Charm is typically quoted per day, so divide by 365
	return charm / 365.0
}

// CalculateAllGreeks computes all 9 Greeks in one call
// This is more efficient than separate calls
func (c *Calculator) CalculateAllGreeks(
	optionType string,
	spotPrice, strikePrice float64,
	expiryDate time.Time,
	sigma float64,
) Greeks {
	// Calculate basic Greeks
	basic := c.Calculate(optionType, spotPrice, strikePrice, expiryDate, sigma)

	// Calculate advanced Greeks
	advanced := c.CalculateAdvanced(optionType, spotPrice, strikePrice,
		expiryDate, sigma)

	// Combine into single struct
	return Greeks{
		Delta:       basic.Delta,
		Gamma:       basic.Gamma,
		Theta:       basic.Theta,
		Vega:        basic.Vega,
		Rho:         basic.Rho,
		ImpliedVol:  sigma, // Passed in (calculated separately)
		IVEstimated: false, // Assumed pre-calculated
		Vanna:       advanced.Vanna,
		Vomma:       advanced.Vomma,
		Charm:       advanced.Charm,
	}
}
