package greeks

import (
	"math"
	"testing"
	"time"
)

func TestNormalCDF(t *testing.T) {
	tests := []struct {
		x        float64
		expected float64
	}{
		{0.0, 0.5},
		{1.0, 0.8413},
		{-1.0, 0.1587},
		{2.0, 0.9772},
		{-2.0, 0.0228},
	}

	for _, tt := range tests {
		result := NormalCDF(tt.x)
		if math.Abs(result-tt.expected) > 0.001 {
			t.Errorf("NormalCDF(%f) = %f, expected %f", tt.x, result, tt.expected)
		}
	}
}

func TestCallDelta(t *testing.T) {
	calc := NewCalculator(0.07)

	// 3 days from now
	expiry := time.Now().AddDate(0, 0, 3)

	greeks := calc.Calculate(
		"CE",
		84733.0, // SENSEX spot
		85500.0, // Strike (OTM)
		expiry,
		0.15, // 15% volatility
	)

	// Call delta should be between 0 and 1
	if greeks.Delta < 0 || greeks.Delta > 1 {
		t.Errorf("Call delta out of range: %f", greeks.Delta)
	}

	// OTM call should have delta < 0.5
	if greeks.Delta >= 0.5 {
		t.Logf("Warning: OTM call delta >= 0.5: %f (might be near ATM)", greeks.Delta)
	}

	t.Logf("Call Delta: %.4f", greeks.Delta)
}

func TestPutDelta(t *testing.T) {
	calc := NewCalculator(0.07)

	expiry := time.Now().AddDate(0, 0, 3)

	greeks := calc.Calculate(
		"PE",
		84733.0,
		85500.0,
		expiry,
		0.15,
	)

	// Put delta should be between -1 and 0
	if greeks.Delta > 0 || greeks.Delta < -1 {
		t.Errorf("Put delta out of range: %f", greeks.Delta)
	}

	t.Logf("Put Delta: %.4f", greeks.Delta)
}

func TestGammaPositive(t *testing.T) {
	calc := NewCalculator(0.07)

	expiry := time.Now().AddDate(0, 0, 3)

	greeks := calc.Calculate("CE", 84733.0, 85500.0, expiry, 0.15)

	// Gamma should always be positive
	if greeks.Gamma <= 0 {
		t.Errorf("Gamma should be positive, got %f", greeks.Gamma)
	}

	t.Logf("Gamma: %.6f", greeks.Gamma)
}

func TestAllGreeks(t *testing.T) {
	calc := NewCalculator(0.07)

	expiry := time.Now().AddDate(0, 0, 3)

	greeks := calc.Calculate("CE", 84733.0, 85500.0, expiry, 0.15)

	t.Logf("All Greeks for SENSEX 85500 CE (3 days to expiry):")
	t.Logf("  Delta: %.4f", greeks.Delta)
	t.Logf("  Gamma: %.6f", greeks.Gamma)
	t.Logf("  Theta: %.2f", greeks.Theta)
	t.Logf("  Vega:  %.2f", greeks.Vega)
	t.Logf("  Rho:   %.2f", greeks.Rho)

	// Validate reasonable ranges
	if greeks.Theta >= 0 {
		t.Logf("Warning: Theta is positive (unusual for long options)")
	}
	if greeks.Vega <= 0 {
		t.Errorf("Vega should be positive for long options")
	}
}

func TestExpiredOption(t *testing.T) {
	calc := NewCalculator(0.07)

	// Expired yesterday
	expiry := time.Now().AddDate(0, 0, -1)

	greeks := calc.Calculate("CE", 84733.0, 85500.0, expiry, 0.15)

	// All Greeks should be zero except Delta
	if greeks.Gamma != 0 || greeks.Theta != 0 ||
		greeks.Vega != 0 || greeks.Rho != 0 {
		t.Errorf("Expired option should have all Greeks zero except Delta")
	}

	t.Logf("Expired OTM Call Delta: %.1f", greeks.Delta)
}

func TestATMOption(t *testing.T) {
	calc := NewCalculator(0.07)

	expiry := time.Now().AddDate(0, 0, 30) // 30 days

	// ATM option (spot = strike)
	greeks := calc.Calculate("CE", 85000.0, 85000.0, expiry, 0.15)

	// ATM call delta should be around 0.5
	if math.Abs(greeks.Delta-0.5) > 0.1 {
		t.Logf("Warning: ATM call delta not near 0.5: %.4f", greeks.Delta)
	}

	// ATM options have highest gamma
	if greeks.Gamma <= 0 {
		t.Errorf("ATM option should have positive gamma")
	}

	t.Logf("ATM Greeks:")
	t.Logf("  Delta: %.4f (should be ~0.5)", greeks.Delta)
	t.Logf("  Gamma: %.6f (highest for ATM)", greeks.Gamma)
}

func TestMoneyness(t *testing.T) {
	tests := []struct {
		optionType  string
		spotPrice   float64
		strikePrice float64
		expected    string
	}{
		{"CE", 85000.0, 85000.0, "ATM"},
		{"CE", 86000.0, 85000.0, "ITM"},
		{"CE", 84000.0, 85000.0, "OTM"},
		{"PE", 85000.0, 85000.0, "ATM"},
		{"PE", 84000.0, 85000.0, "ITM"},
		{"PE", 86000.0, 85000.0, "OTM"},
	}

	for _, tt := range tests {
		result := Moneyness(tt.optionType, tt.spotPrice, tt.strikePrice)
		if result != tt.expected {
			t.Errorf("Moneyness(%s, %.0f, %.0f) = %s, expected %s",
				tt.optionType, tt.spotPrice, tt.strikePrice, result, tt.expected)
		}
	}
}

func TestIntrinsicValue(t *testing.T) {
	tests := []struct {
		optionType  string
		spotPrice   float64
		strikePrice float64
		expected    float64
	}{
		{"CE", 86000.0, 85000.0, 1000.0}, // ITM call
		{"CE", 84000.0, 85000.0, 0.0},    // OTM call
		{"PE", 84000.0, 85000.0, 1000.0}, // ITM put
		{"PE", 86000.0, 85000.0, 0.0},    // OTM put
	}

	for _, tt := range tests {
		result := IntrinsicValue(tt.optionType, tt.spotPrice, tt.strikePrice)
		if result != tt.expected {
			t.Errorf("IntrinsicValue(%s, %.0f, %.0f) = %.0f, expected %.0f",
				tt.optionType, tt.spotPrice, tt.strikePrice, result, tt.expected)
		}
	}
}

func BenchmarkGreeksCalculation(b *testing.B) {
	calc := NewCalculator(0.07)
	expiry := time.Now().AddDate(0, 0, 3)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		calc.Calculate("CE", 84733.0, 85500.0, expiry, 0.15)
	}
}

func BenchmarkNormalCDF(b *testing.B) {
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		NormalCDF(0.5)
	}
}
