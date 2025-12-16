package greeks

import (
	"errors"
	"fmt"
	"math"
	"time"
)

// IVConfig holds configuration for IV calculation
type IVConfig struct {
	InitialGuess  float64 // Starting volatility guess (e.g., 0.20 = 20%)
	MaxIterations int     // Maximum Newton-Raphson iterations
	Tolerance     float64 // Price convergence tolerance (e.g., 0.0001 = ₹0.01)
	MinVol        float64 // Minimum allowed volatility (e.g., 0.01 = 1%)
	MaxVol        float64 // Maximum allowed volatility (e.g., 3.0 = 300%)
}

// DefaultIVConfig returns sensible defaults for IV calculation
func DefaultIVConfig() IVConfig {
	return IVConfig{
		InitialGuess:  0.10, // 10% volatility (better for short-term options)
		MaxIterations: 100,  // Enough for most cases
		Tolerance:     0.01, // ₹0.01 accuracy (tighter tolerance)
		MinVol:        0.01, // 1% floor
		MaxVol:        2.00, // 200% cap (more reasonable)
	}
}

// ImpliedVolatility calculates IV using Newton-Raphson method
// It finds the volatility that makes Black-Scholes price equal market price
func (c *Calculator) ImpliedVolatility(
	marketPrice float64,
	optionType string,
	spotPrice float64,
	strikePrice float64,
	expiryDate time.Time,
	config IVConfig,
) (float64, error) {
	// Input validation
	if marketPrice <= 0 {
		return 0, errors.New("market price must be positive")
	}
	if spotPrice <= 0 {
		return 0, errors.New("spot price must be positive")
	}
	if strikePrice <= 0 {
		return 0, errors.New("strike price must be positive")
	}

	// Calculate time to expiry
	timeToExpiry := time.Until(expiryDate).Hours() / (24 * 365.25)
	if timeToExpiry <= 0 {
		// Expired option - return intrinsic value check
		intrinsic := c.calculateIntrinsicValue(optionType, spotPrice, strikePrice)
		if math.Abs(marketPrice-intrinsic) < 0.01 {
			return 0, nil // At intrinsic, zero time value
		}
		return 0, errors.New("option has expired")
	}

	// Check intrinsic value bounds
	intrinsic := c.calculateIntrinsicValue(optionType, spotPrice, strikePrice)
	if marketPrice < intrinsic-0.01 { // Allow small tolerance
		return 0, fmt.Errorf("market price (%.2f) below intrinsic value (%.2f)", marketPrice, intrinsic)
	}

	// Adjust initial guess based on moneyness
	sigma := c.getInitialVolGuess(optionType, spotPrice, strikePrice, config.InitialGuess)

	// Newton-Raphson iteration
	for i := 0; i < config.MaxIterations; i++ {
		// Calculate BS price with current sigma
		bsPrice := c.calculatePrice(optionType, spotPrice, strikePrice, timeToExpiry, sigma)

		// Check convergence
		diff := bsPrice - marketPrice
		if math.Abs(diff) < config.Tolerance {
			return sigma, nil
		}

		// Calculate Vega directly (faster than full Greeks calculation)
		sqrtT := math.Sqrt(timeToExpiry)
		d1 := (math.Log(spotPrice/strikePrice) +
			(c.riskFreeRate+0.5*sigma*sigma)*timeToExpiry) / (sigma * sqrtT)

		// Vega = S * N'(d1) * sqrt(T)
		// N'(d1) is the standard normal PDF
		nd1 := math.Exp(-0.5*d1*d1) / math.Sqrt(2*math.Pi)
		vega := spotPrice * nd1 * sqrtT

		if vega < 1e-10 {
			return 0, errors.New("vega too small, cannot converge")
		}

		// Newton-Raphson update: σ_new = σ_old - (BS_Price - Market_Price) / Vega
		// Note: Vega here is the actual mathematical vega (change per unit vol change)
		sigma = sigma - (diff / vega)

		// Apply bounds
		if sigma < config.MinVol {
			sigma = config.MinVol
		}
		if sigma > config.MaxVol {
			sigma = config.MaxVol
		}
	}

	return 0, fmt.Errorf("IV did not converge after %d iterations (last diff: %.4f)",
		config.MaxIterations,
		c.calculatePrice(optionType, spotPrice, strikePrice, timeToExpiry, sigma)-marketPrice)
}

// calculatePrice computes Black-Scholes option price
func (c *Calculator) calculatePrice(
	optionType string,
	spotPrice, strikePrice, timeToExpiry, sigma float64,
) float64 {
	if timeToExpiry <= 0 {
		// Expired - return intrinsic value
		return c.calculateIntrinsicValue(optionType, spotPrice, strikePrice)
	}

	if sigma <= 0 {
		// Zero volatility - return discounted intrinsic
		return c.calculateIntrinsicValue(optionType, spotPrice, strikePrice) *
			math.Exp(-c.riskFreeRate*timeToExpiry)
	}

	// Calculate d1 and d2
	sqrtT := math.Sqrt(timeToExpiry)
	d1 := (math.Log(spotPrice/strikePrice) +
		(c.riskFreeRate+0.5*sigma*sigma)*timeToExpiry) / (sigma * sqrtT)
	d2 := d1 - sigma*sqrtT

	// Discount factor
	discountFactor := math.Exp(-c.riskFreeRate * timeToExpiry)

	if optionType == "CE" {
		// Call price: S×N(d1) - K×e^(-rt)×N(d2)
		return spotPrice*NormalCDF(d1) - strikePrice*discountFactor*NormalCDF(d2)
	} else {
		// Put price: K×e^(-rt)×N(-d2) - S×N(-d1)
		return strikePrice*discountFactor*NormalCDF(-d2) - spotPrice*NormalCDF(-d1)
	}
}

// calculateIntrinsicValue returns intrinsic value (payoff at expiry)
func (c *Calculator) calculateIntrinsicValue(
	optionType string,
	spotPrice, strikePrice float64,
) float64 {
	if optionType == "CE" {
		// Call intrinsic: max(S - K, 0)
		return math.Max(spotPrice-strikePrice, 0)
	} else {
		// Put intrinsic: max(K - S, 0)
		return math.Max(strikePrice-spotPrice, 0)
	}
}

// getInitialVolGuess returns smarter initial volatility guess based on moneyness
func (c *Calculator) getInitialVolGuess(
	optionType string,
	spotPrice, strikePrice, defaultGuess float64,
) float64 {
	// Calculate moneyness
	moneyness := spotPrice / strikePrice

	// ATM options (0.95 < S/K < 1.05): use default guess
	if moneyness > 0.95 && moneyness < 1.05 {
		return defaultGuess
	}

	// OTM options tend to have higher IV (volatility smile)
	if optionType == "CE" {
		// Call OTM when S < K
		if moneyness < 0.95 {
			return defaultGuess * 1.2 // 20% higher
		}
	} else {
		// Put OTM when S > K
		if moneyness > 1.05 {
			return defaultGuess * 1.2
		}
	}

	// ITM options tend to have lower IV
	return defaultGuess * 0.9
}

// ImpliedVolatilityWithFallback tries to calculate IV, falls back to default if fails
func (c *Calculator) ImpliedVolatilityWithFallback(
	marketPrice float64,
	optionType string,
	spotPrice float64,
	strikePrice float64,
	expiryDate time.Time,
	config IVConfig,
	fallbackVol float64,
) (iv float64, estimated bool) {
	iv, err := c.ImpliedVolatility(marketPrice, optionType, spotPrice,
		strikePrice, expiryDate, config)

	if err != nil {
		// Failed to converge - use fallback
		return fallbackVol, true
	}

	return iv, false
}

// ImpliedVolatilityFromMidPrice calculates IV using bid-ask midpoint
// Useful when LTP is stale or zero
func (c *Calculator) ImpliedVolatilityFromMidPrice(
	bidPrice, askPrice float64,
	optionType string,
	spotPrice float64,
	strikePrice float64,
	expiryDate time.Time,
	config IVConfig,
) (float64, error) {
	if bidPrice <= 0 || askPrice <= 0 {
		return 0, errors.New("invalid bid/ask prices")
	}

	midPrice := (bidPrice + askPrice) / 2.0
	return c.ImpliedVolatility(midPrice, optionType, spotPrice,
		strikePrice, expiryDate, config)
}
