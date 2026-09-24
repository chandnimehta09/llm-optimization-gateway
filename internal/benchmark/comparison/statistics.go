package comparison

import (
	"math"
)

// WelchTTest performs a Welch's t-test to determine if two samples have
// significantly different means. Returns whether the difference is significant
// at the given alpha level and the p-value approximation.
func WelchTTest(sample1, sample2 []float64, alpha float64) (significant bool, pValue float64) {
	n1 := float64(len(sample1))
	n2 := float64(len(sample2))

	if n1 < 2 || n2 < 2 {
		return false, 1.0
	}

	mean1 := Mean(sample1)
	mean2 := Mean(sample2)
	var1 := Variance(sample1)
	var2 := Variance(sample2)

	// Welch's t-statistic.
	se := math.Sqrt(var1/n1 + var2/n2)
	if se == 0 {
		if mean1 == mean2 {
			return false, 1.0
		}
		return true, 0.0
	}

	t := (mean1 - mean2) / se

	// Welch-Satterthwaite degrees of freedom.
	num := math.Pow(var1/n1+var2/n2, 2)
	den := math.Pow(var1/n1, 2)/(n1-1) + math.Pow(var2/n2, 2)/(n2-1)
	if den == 0 {
		return false, 1.0
	}
	df := num / den

	// Approximate p-value using the t-distribution.
	pValue = approximatePValue(math.Abs(t), df)

	return pValue < alpha, pValue
}

// Mean computes the arithmetic mean of a sample.
func Mean(sample []float64) float64 {
	if len(sample) == 0 {
		return 0
	}
	sum := 0.0
	for _, v := range sample {
		sum += v
	}
	return sum / float64(len(sample))
}

// Variance computes the sample variance (Bessel's correction).
func Variance(sample []float64) float64 {
	if len(sample) < 2 {
		return 0
	}
	m := Mean(sample)
	sum := 0.0
	for _, v := range sample {
		d := v - m
		sum += d * d
	}
	return sum / float64(len(sample)-1)
}

// StdDev computes the sample standard deviation.
func StdDev(sample []float64) float64 {
	return math.Sqrt(Variance(sample))
}

// Percentile computes the p-th percentile of a sorted sample.
// p should be between 0 and 1.
func Percentile(sorted []float64, p float64) float64 {
	if len(sorted) == 0 {
		return 0
	}
	idx := p * float64(len(sorted)-1)
	lower := int(math.Floor(idx))
	upper := int(math.Ceil(idx))
	if lower == upper || upper >= len(sorted) {
		return sorted[lower]
	}
	frac := idx - float64(lower)
	return sorted[lower]*(1-frac) + sorted[upper]*frac
}

// ConfidenceInterval computes the confidence interval for the mean difference.
func ConfidenceInterval(sample1, sample2 []float64, confidence float64) (lower, upper float64) {
	n1 := float64(len(sample1))
	n2 := float64(len(sample2))
	if n1 < 2 || n2 < 2 {
		return 0, 0
	}

	mean1 := Mean(sample1)
	mean2 := Mean(sample2)
	var1 := Variance(sample1)
	var2 := Variance(sample2)

	diff := mean1 - mean2
	se := math.Sqrt(var1/n1 + var2/n2)

	// Use z-score approximation for simplicity.
	z := zScore(confidence)
	margin := z * se

	return diff - margin, diff + margin
}

// EffectSize computes Cohen's d effect size between two samples.
func EffectSize(sample1, sample2 []float64) float64 {
	if len(sample1) < 2 || len(sample2) < 2 {
		return 0
	}

	mean1 := Mean(sample1)
	mean2 := Mean(sample2)
	var1 := Variance(sample1)
	var2 := Variance(sample2)

	// Pooled standard deviation.
	n1 := float64(len(sample1))
	n2 := float64(len(sample2))
	pooledVar := ((n1-1)*var1 + (n2-1)*var2) / (n1 + n2 - 2)
	pooledSD := math.Sqrt(pooledVar)

	if pooledSD == 0 {
		return 0
	}

	return (mean1 - mean2) / pooledSD
}

// EffectSizeLabel returns a human-readable label for Cohen's d.
func EffectSizeLabel(d float64) string {
	absD := math.Abs(d)
	switch {
	case absD < 0.2:
		return "negligible"
	case absD < 0.5:
		return "small"
	case absD < 0.8:
		return "medium"
	default:
		return "large"
	}
}

// approximatePValue approximates the two-tailed p-value from a t-distribution.
// Uses a simple approximation based on the normal distribution for large df.
func approximatePValue(t, df float64) float64 {
	if df <= 0 {
		return 1.0
	}
	// For large df, t-distribution approaches normal.
	// Use a rational approximation of the normal CDF.
	x := t * (1 - 1/(4*df)) / math.Sqrt(1+t*t/(2*df))
	p := 2 * normalCDFComplement(x)
	if p > 1 {
		return 1.0
	}
	if p < 0 {
		return 0.0
	}
	return p
}

// normalCDFComplement returns 1 - Phi(x) using an approximation.
func normalCDFComplement(x float64) float64 {
	return 0.5 * math.Erfc(x/math.Sqrt2)
}

// zScore returns the z-score for a given confidence level.
func zScore(confidence float64) float64 {
	switch {
	case confidence >= 0.99:
		return 2.576
	case confidence >= 0.95:
		return 1.960
	case confidence >= 0.90:
		return 1.645
	default:
		return 1.960
	}
}
