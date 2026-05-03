// Package stats provides statistical significance testing for benchmark comparisons.
// Pure Go, no external dependencies.
package stats

import (
	"math"
	"math/rand/v2"
	"sort"
)

// Sample holds a collection of observations with precomputed summary statistics.
type Sample struct {
	Values []float64
	N      int
	Mean   float64
	StdDev float64
}

// NewSample creates a Sample from raw values, computing summary stats.
func NewSample(values []float64) Sample {
	n := len(values)
	if n == 0 {
		return Sample{}
	}

	mean := 0.0
	for _, v := range values {
		mean += v
	}
	mean /= float64(n)

	variance := 0.0
	for _, v := range values {
		d := v - mean
		variance += d * d
	}
	if n > 1 {
		variance /= float64(n - 1) // Bessel's correction
	}

	return Sample{
		Values: values,
		N:      n,
		Mean:   mean,
		StdDev: math.Sqrt(variance),
	}
}

// TTestResult holds the output of a Welch's t-test.
type TTestResult struct {
	T      float64 // t-statistic
	DF     float64 // degrees of freedom (Welch-Satterthwaite)
	PValue float64 // two-tailed p-value
}

// WelchTTest performs Welch's t-test (unequal variances) on two independent samples.
// Returns the t-statistic, degrees of freedom, and two-tailed p-value.
func WelchTTest(a, b Sample) TTestResult {
	if a.N < 2 || b.N < 2 {
		return TTestResult{PValue: 1.0}
	}

	varA := a.StdDev * a.StdDev
	varB := b.StdDev * b.StdDev
	nA := float64(a.N)
	nB := float64(b.N)

	// Standard error of the difference
	se := math.Sqrt(varA/nA + varB/nB)
	if se == 0 {
		return TTestResult{PValue: 1.0}
	}

	t := (a.Mean - b.Mean) / se

	// Welch-Satterthwaite degrees of freedom
	num := (varA/nA + varB/nB) * (varA/nA + varB/nB)
	denom := (varA*varA)/(nA*nA*(nA-1)) + (varB*varB)/(nB*nB*(nB-1))
	if denom == 0 {
		return TTestResult{T: t, PValue: 1.0}
	}
	df := num / denom

	// Two-tailed p-value from t-distribution
	p := 2.0 * tDistCDF(-math.Abs(t), df)

	return TTestResult{T: t, DF: df, PValue: p}
}

// ConfidenceInterval represents a confidence interval.
type ConfidenceInterval struct {
	Lower      float64
	Upper      float64
	Confidence float64 // e.g. 0.95
}

// BootstrapCI computes a bootstrap confidence interval for the mean difference (a - b).
// nBoot is the number of bootstrap resamples. confidence is typically 0.95.
func BootstrapCI(a, b Sample, nBoot int, confidence float64) ConfidenceInterval {
	if a.N == 0 || b.N == 0 || nBoot <= 0 {
		return ConfidenceInterval{Confidence: confidence}
	}

	diffs := make([]float64, nBoot)
	for i := range nBoot {
		_ = i
		meanA := bootstrapMean(a.Values)
		meanB := bootstrapMean(b.Values)
		diffs[i] = meanA - meanB
	}

	sort.Float64s(diffs)

	alpha := 1.0 - confidence
	lowerIdx := int(math.Floor(alpha / 2.0 * float64(nBoot)))
	upperIdx := int(math.Ceil((1.0 - alpha/2.0) * float64(nBoot)))
	if lowerIdx < 0 {
		lowerIdx = 0
	}
	if upperIdx >= nBoot {
		upperIdx = nBoot - 1
	}

	return ConfidenceInterval{
		Lower:      diffs[lowerIdx],
		Upper:      diffs[upperIdx],
		Confidence: confidence,
	}
}

func bootstrapMean(values []float64) float64 {
	n := len(values)
	sum := 0.0
	for range n {
		sum += values[rand.IntN(n)]
	}
	return sum / float64(n)
}

// EffectSizeResult holds Cohen's d and interpretation.
type EffectSizeResult struct {
	D              float64 // Cohen's d
	Interpretation string  // "negligible", "small", "medium", "large"
}

// EffectSize computes Cohen's d for the difference between two samples.
// Uses pooled standard deviation.
func EffectSize(a, b Sample) EffectSizeResult {
	if a.N < 2 || b.N < 2 {
		return EffectSizeResult{D: 0, Interpretation: "negligible"}
	}

	// Pooled standard deviation
	nA := float64(a.N)
	nB := float64(b.N)
	pooledVar := ((nA-1)*a.StdDev*a.StdDev + (nB-1)*b.StdDev*b.StdDev) / (nA + nB - 2)
	pooledSD := math.Sqrt(pooledVar)

	if pooledSD == 0 {
		return EffectSizeResult{D: 0, Interpretation: "negligible"}
	}

	d := (a.Mean - b.Mean) / pooledSD

	var interp string
	absD := math.Abs(d)
	switch {
	case absD < 0.2:
		interp = "negligible"
	case absD < 0.5:
		interp = "small"
	case absD < 0.8:
		interp = "medium"
	default:
		interp = "large"
	}

	return EffectSizeResult{D: d, Interpretation: interp}
}

// MinSampleSize estimates the minimum sample size per group needed to detect
// a given effect size (Cohen's d) with the specified power and significance level.
// Uses the approximation: n = ((z_alpha + z_beta) / d)^2 + 1
func MinSampleSize(effectSize, alpha, power float64) int {
	if effectSize <= 0 || alpha <= 0 || alpha >= 1 || power <= 0 || power >= 1 {
		return 0
	}

	zAlpha := normalQuantile(1.0 - alpha/2.0)
	zBeta := normalQuantile(power)

	n := math.Ceil((zAlpha+zBeta)/effectSize*((zAlpha+zBeta)/effectSize)) + 1
	if n < 2 {
		n = 2
	}
	return int(n)
}

// IsSignificant returns true if p < alpha and the effect size is at least "small".
func IsSignificant(pValue, alpha float64, effect EffectSizeResult) bool {
	return pValue < alpha && effect.Interpretation != "negligible"
}

// --- internal math helpers (no external deps) ---

// tDistCDF approximates the CDF of Student's t-distribution using the
// regularized incomplete beta function.
func tDistCDF(t, df float64) float64 {
	if df <= 0 {
		return 0.5
	}
	x := df / (df + t*t)
	beta := regIncBeta(df/2.0, 0.5, x)
	if t < 0 {
		return 0.5 * beta
	}
	return 1.0 - 0.5*beta
}

// regIncBeta computes the regularized incomplete beta function I_x(a, b)
// using a continued fraction expansion.
func regIncBeta(a, b, x float64) float64 {
	if x < 0 || x > 1 {
		return 0
	}
	if x == 0 || x == 1 {
		return x
	}

	lbeta := lgamma(a+b) - lgamma(a) - lgamma(b)
	prefix := math.Exp(lbeta+a*math.Log(x)+b*math.Log(1-x)) / a

	// Use Lentz's continued fraction
	if x < (a+1)/(a+b+2) {
		return prefix * betaCF(a, b, x)
	}
	return 1.0 - (math.Exp(lbeta+a*math.Log(x)+b*math.Log(1-x))/b)*betaCF(b, a, 1-x)
}

func betaCF(a, b, x float64) float64 {
	const maxIter = 200
	const eps = 1e-14

	qab := a + b
	qap := a + 1
	qam := a - 1
	c := 1.0
	d := 1.0 - qab*x/qap
	if math.Abs(d) < 1e-30 {
		d = 1e-30
	}
	d = 1.0 / d
	h := d

	for m := 1; m <= maxIter; m++ {
		mf := float64(m)
		m2 := 2.0 * mf

		// Even step
		aa := mf * (b - mf) * x / ((qam + m2) * (a + m2))
		d = 1.0 + aa*d
		if math.Abs(d) < 1e-30 {
			d = 1e-30
		}
		c = 1.0 + aa/c
		if math.Abs(c) < 1e-30 {
			c = 1e-30
		}
		d = 1.0 / d
		h *= d * c

		// Odd step
		aa = -(a + mf) * (qab + mf) * x / ((a + m2) * (qap + m2))
		d = 1.0 + aa*d
		if math.Abs(d) < 1e-30 {
			d = 1e-30
		}
		c = 1.0 + aa/c
		if math.Abs(c) < 1e-30 {
			c = 1e-30
		}
		d = 1.0 / d
		delta := d * c
		h *= delta

		if math.Abs(delta-1.0) < eps {
			break
		}
	}

	return h
}

func lgamma(x float64) float64 {
	v, _ := math.Lgamma(x)
	return v
}

// normalQuantile approximates the quantile (inverse CDF) of the standard normal
// distribution using the rational approximation from Abramowitz & Stegun.
func normalQuantile(p float64) float64 {
	if p <= 0 {
		return math.Inf(-1)
	}
	if p >= 1 {
		return math.Inf(1)
	}
	if p == 0.5 {
		return 0
	}

	// Rational approximation
	t := 0.0
	if p < 0.5 {
		t = math.Sqrt(-2.0 * math.Log(p))
	} else {
		t = math.Sqrt(-2.0 * math.Log(1.0-p))
	}

	c0 := 2.515517
	c1 := 0.802853
	c2 := 0.010328
	d1 := 1.432788
	d2 := 0.189269
	d3 := 0.001308

	result := t - (c0+c1*t+c2*t*t)/(1.0+d1*t+d2*t*t+d3*t*t*t)
	if p < 0.5 {
		return -result
	}
	return result
}
