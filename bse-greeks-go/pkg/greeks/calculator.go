package greeks

import (
	"math"
	"time"
)

// Greeks contains all 9 option Greeks (basic + advanced + IV)
type Greeks struct {
	// Basic Greeks (First-order derivatives)
	Delta float64 `json:"delta"` // Directional risk (0 to 1 for calls, -1 to 0 for puts)
	Gamma float64 `json:"gamma"` // Delta sensitivity (always positive)
	Theta float64 `json:"theta"` // Time decay per day (usually negative)
	Vega  float64 `json:"vega"`  // Volatility sensitivity (always positive)
	Rho   float64 `json:"rho"`   // Interest rate sensitivity

	// Implied Volatility (Calculated from market price)
	ImpliedVol  float64 `json:"implied_vol"`  // Market-implied volatility
	IVEstimated bool    `json:"iv_estimated"` // True if IV calculation failed, using fallback

	// Advanced Greeks (Second-order and cross derivatives)
	Vanna float64 `json:"vanna"` // ∂²V/∂S∂σ - Delta sensitivity to volatility
	Vomma float64 `json:"vomma"` // ∂²V/∂σ² - Vega sensitivity to volatility
	Charm float64 `json:"charm"` // ∂²V/∂S∂t - Delta decay per day
}

// Calculator handles Greeks calculations
type Calculator struct {
	riskFreeRate float64 // Annual risk-free rate (e.g., 0.07 for 7%)
}

// NewCalculator creates a new Greeks calculator
func NewCalculator(riskFreeRate float64) *Calculator {
	return &Calculator{
		riskFreeRate: riskFreeRate,
	}
}

// Calculate computes all Greeks for an option
func (c *Calculator) Calculate(
	optionType string, // "CE" for call, "PE" for put
	spotPrice float64, // Current underlying price (SENSEX/BANKEX)
	strikePrice float64, // Option strike price
	expiryDate time.Time, // Option expiry date
	volatility float64, // Annual volatility (e.g., 0.15 for 15%)
) Greeks {

	// Calculate time to expiry in years
	t := c.timeToExpiry(expiryDate)

	// Handle expired options
	if t <= 0 {
		return Greeks{
			Delta: c.expiredDelta(optionType, spotPrice, strikePrice),
			Gamma: 0,
			Theta: 0,
			Vega:  0,
			Rho:   0,
		}
	}

	// Calculate Black-Scholes d1 and d2
	d1 := c.calculateD1(spotPrice, strikePrice, volatility, t)
	d2 := d1 - volatility*math.Sqrt(t)

	// Calculate Greeks
	greeks := Greeks{}

	// Delta
	if optionType == "CE" {
		greeks.Delta = NormalCDF(d1)
	} else {
		greeks.Delta = NormalCDF(d1) - 1
	}

	// Gamma (same for calls and puts)
	if volatility > 0 && t > 0 {
		greeks.Gamma = NormalPDF(d1) / (spotPrice * volatility * math.Sqrt(t))
	}

	// Theta
	if optionType == "CE" {
		greeks.Theta = c.callTheta(spotPrice, strikePrice, volatility, t, d1, d2)
	} else {
		greeks.Theta = c.putTheta(spotPrice, strikePrice, volatility, t, d1, d2)
	}

	// Vega (same for calls and puts)
	if t > 0 {
		greeks.Vega = spotPrice * NormalPDF(d1) * math.Sqrt(t) / 100
	}

	// Rho
	if optionType == "CE" {
		greeks.Rho = strikePrice * t * math.Exp(-c.riskFreeRate*t) * NormalCDF(d2) / 100
	} else {
		greeks.Rho = -strikePrice * t * math.Exp(-c.riskFreeRate*t) * NormalCDF(-d2) / 100
	}

	return greeks
}

// calculateD1 computes the d1 parameter for Black-Scholes
func (c *Calculator) calculateD1(
	spotPrice float64,
	strikePrice float64,
	volatility float64,
	t float64,
) float64 {
	if t <= 0 || volatility <= 0 || strikePrice <= 0 {
		return 0
	}

	numerator := math.Log(spotPrice/strikePrice) +
		(c.riskFreeRate+0.5*volatility*volatility)*t
	denominator := volatility * math.Sqrt(t)

	if denominator == 0 {
		return 0
	}

	return numerator / denominator
}

// timeToExpiry calculates time to expiry in years
func (c *Calculator) timeToExpiry(expiryDate time.Time) float64 {
	now := time.Now()
	if expiryDate.Before(now) {
		return 0
	}
	duration := expiryDate.Sub(now)
	return duration.Hours() / (24 * 365)
}

// callTheta calculates Theta for call options (per day)
func (c *Calculator) callTheta(
	spotPrice, strikePrice, volatility, t, d1, d2 float64,
) float64 {
	if t <= 0 {
		return 0
	}

	term1 := -(spotPrice * NormalPDF(d1) * volatility) / (2 * math.Sqrt(t))
	term2 := c.riskFreeRate * strikePrice * math.Exp(-c.riskFreeRate*t) * NormalCDF(d2)

	// Return daily theta (divide by 365)
	return (term1 - term2) / 365
}

// putTheta calculates Theta for put options (per day)
func (c *Calculator) putTheta(
	spotPrice, strikePrice, volatility, t, d1, d2 float64,
) float64 {
	if t <= 0 {
		return 0
	}

	term1 := -(spotPrice * NormalPDF(d1) * volatility) / (2 * math.Sqrt(t))
	term2 := c.riskFreeRate * strikePrice * math.Exp(-c.riskFreeRate*t) * NormalCDF(-d2)

	// Return daily theta (divide by 365)
	return (term1 + term2) / 365
}

// expiredDelta returns Delta for expired options
func (c *Calculator) expiredDelta(optionType string, spotPrice, strikePrice float64) float64 {
	if optionType == "CE" {
		if spotPrice > strikePrice {
			return 1.0 // ITM call
		}
		return 0.0 // OTM call
	} else {
		if spotPrice < strikePrice {
			return -1.0 // ITM put
		}
		return 0.0 // OTM put
	}
}

// Moneyness returns the moneyness classification
func Moneyness(optionType string, spotPrice, strikePrice float64) string {
	// Calculate percentage difference
	diff := math.Abs((spotPrice - strikePrice) / strikePrice)
	threshold := 0.02 // 2% threshold for ATM

	// Check ATM first
	if diff < threshold {
		return "ATM"
	}

	// Determine ITM/OTM
	if optionType == "CE" {
		if spotPrice > strikePrice {
			return "ITM"
		}
		return "OTM"
	} else { // PE
		if spotPrice < strikePrice {
			return "ITM"
		}
		return "OTM"
	}
}

// IntrinsicValue calculates the intrinsic value of an option
func IntrinsicValue(optionType string, spotPrice, strikePrice float64) float64 {
	if optionType == "CE" {
		return math.Max(spotPrice-strikePrice, 0)
	}
	return math.Max(strikePrice-spotPrice, 0)
}

// CalculateWithIV calculates all 9 Greeks including IV from market price
// This is the main method to use when you have market data
func (c *Calculator) CalculateWithIV(
	marketPrice float64,
	optionType string,
	spotPrice float64,
	strikePrice float64,
	expiryDate time.Time,
	volume int64, // Used to determine if price is reliable
) Greeks {
	// Default IV config
	ivConfig := DefaultIVConfig()

	// Step 1: Calculate Implied Volatility from market price
	iv, ivEstimated := c.ImpliedVolatilityWithFallback(
		marketPrice,
		optionType,
		spotPrice,
		strikePrice,
		expiryDate,
		ivConfig,
		0.15, // 15% fallback volatility
	)

	// If volume is very low, mark as estimated
	if volume < 10 {
		ivEstimated = true
	}

	// Step 2: Calculate all Greeks using the calculated IV
	return c.CalculateAllGreeksWithIV(optionType, spotPrice, strikePrice,
		expiryDate, iv, ivEstimated)
}

// CalculateAllGreeksWithIV computes all 9 Greeks with pre-calculated IV
func (c *Calculator) CalculateAllGreeksWithIV(
	optionType string,
	spotPrice, strikePrice float64,
	expiryDate time.Time,
	iv float64,
	ivEstimated bool,
) Greeks {
	// Calculate basic Greeks
	basic := c.Calculate(optionType, spotPrice, strikePrice, expiryDate, iv)

	// Calculate advanced Greeks
	advanced := c.CalculateAdvanced(optionType, spotPrice, strikePrice,
		expiryDate, iv)

	// Combine into single struct
	return Greeks{
		Delta:       basic.Delta,
		Gamma:       basic.Gamma,
		Theta:       basic.Theta,
		Vega:        basic.Vega,
		Rho:         basic.Rho,
		ImpliedVol:  iv,
		IVEstimated: ivEstimated,
		Vanna:       advanced.Vanna,
		Vomma:       advanced.Vomma,
		Charm:       advanced.Charm,
	}
}

// TimeValue calculates the time value of an option
func TimeValue(ltp, intrinsicValue float64) float64 {
	return math.Max(ltp-intrinsicValue, 0)
}
