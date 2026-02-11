package predict

import (
	"hash/fnv"
	"math"
	"math/rand/v2"
	"strconv"
	"strings"
)

type Config struct {
	Seed int64 // 0 = deterministic per-input hashing
}

type Predictor struct {
	seed int64
}

func New(cfg Config) *Predictor {
	return &Predictor{seed: cfg.Seed}
}

type Target struct {
	TargetCents      int64
	FirstOfferCents  int64
	WalkAwayCents    int64
	Confidence       float64
	DiscountFraction float64
}

func (p *Predictor) Predict(baselineCents int64, qty float64, supplierCount int) Target {
	if baselineCents < 0 {
		baselineCents = 0
	}
	if qty < 0 {
		qty = 0
	}
	if supplierCount < 0 {
		supplierCount = 0
	}

	discount := 0.08
	// Quantity helps, but diminishing returns.
	discount += 0.03 * math.Log10(qty+1)
	// More suppliers increases competition a bit.
	discount += math.Min(0.08, float64(supplierCount)*0.01)

	// Add a small jitter so items don't all look identical.
	discount += 0.02 * (p.jitter(baselineCents, qty, supplierCount) - 0.5)

	if discount < 0.03 {
		discount = 0.03
	}
	if discount > 0.30 {
		discount = 0.30
	}

	target := int64(math.Round(float64(baselineCents) * (1.0 - discount)))
	walkAway := int64(math.Round(float64(baselineCents) * 0.98))
	firstOffer := int64(math.Round(float64(target) * 0.97))

	confidence := 0.55 + 0.15*math.Tanh(math.Log10(qty+1)/2)
	confidence += math.Min(0.15, float64(supplierCount)*0.02)
	if confidence > 0.92 {
		confidence = 0.92
	}

	return Target{
		TargetCents:      max64(0, target),
		FirstOfferCents:  max64(0, firstOffer),
		WalkAwayCents:    max64(0, walkAway),
		Confidence:       confidence,
		DiscountFraction: discount,
	}
}

// ModelBaseline provides a deterministic (or seeded) baseline unit price for cases
// where the buyer doesn't have a recent price. This is intentionally simple: it
// exists to make the demo usable without requiring full spend history.
func (p *Predictor) ModelBaseline(name, category string, qty float64) int64 {
	name = strings.ToLower(strings.TrimSpace(name))
	category = strings.ToLower(strings.TrimSpace(category))
	if qty < 0 {
		qty = 0
	}

	// Base price in [$5, $500) chosen by stable hash.
	base := int64(500 + p.hashRange("baseline", name+"|"+category, 49_500))

	// Quantity lowers the baseline a bit, with diminishing returns.
	factor := 1.0 / (1.0 + 0.12*math.Log10(qty+1))
	modeled := int64(math.Round(float64(base) * factor))
	if modeled < 50 {
		modeled = 50
	}
	return modeled
}

func (p *Predictor) jitter(baselineCents int64, qty float64, supplierCount int) float64 {
	if p.seed != 0 {
		r := rand.New(rand.NewPCG(uint64(p.seed), uint64(baselineCents)))
		return r.Float64()
	}

	h := fnv.New64a()
	_, _ = h.Write([]byte("arkessro"))
	_, _ = h.Write([]byte{0})
	_, _ = h.Write([]byte(floatKey(qty)))
	_, _ = h.Write([]byte{0})
	_, _ = h.Write([]byte(intKey(supplierCount)))
	_, _ = h.Write([]byte{0})
	_, _ = h.Write([]byte(int64Key(baselineCents)))

	v := h.Sum64()
	// Map to [0,1).
	return float64(v%10_000) / 10_000.0
}

func (p *Predictor) hashRange(ns, s string, mod int64) int64 {
	if mod <= 0 {
		return 0
	}
	if p.seed != 0 {
		r := rand.New(rand.NewPCG(uint64(p.seed), uint64(fnv64(ns, s))))
		return int64(r.Uint64() % uint64(mod))
	}
	return int64(fnv64(ns, s) % uint64(mod))
}

func fnv64(ns, s string) uint64 {
	h := fnv.New64a()
	_, _ = h.Write([]byte("arkessro"))
	_, _ = h.Write([]byte{0})
	_, _ = h.Write([]byte(ns))
	_, _ = h.Write([]byte{0})
	_, _ = h.Write([]byte(s))
	return h.Sum64()
}

func floatKey(v float64) string {
	// Stable-ish for our use; not for general purpose hashing.
	return strconv.FormatFloat(v, 'g', 12, 64)
}

func intKey(v int) string     { return strconv.FormatInt(int64(v), 10) }
func int64Key(v int64) string { return strconv.FormatInt(v, 10) }

func max64(a, b int64) int64 {
	if a > b {
		return a
	}
	return b
}
