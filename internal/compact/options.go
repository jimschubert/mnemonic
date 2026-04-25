package compact

import (
	"log/slog"
)

// CavemanMode options to cover Julius Brussee's caveman mode for more aggressive compacting.
// see: https://juliusbrussee.github.io/caveman/
// License MIT: https://github.com/JuliusBrussee/caveman/blob/main/LICENSE
type CavemanMode int

const (
	// CavemanOff is the default mode uses a normal prompt.
	CavemanOff CavemanMode = iota
	// CavemanLite is: No filler/hedging. Keep articles + full sentences. Professional but tight.
	CavemanLite
	// CavemanFull is: Drop articles, fragments OK, short synonyms. Classic caveman.
	CavemanFull
	// CavemanUltra is the most aggressive mode: Abbreviate (DB/auth/config/req/res/fn/impl), strip conjunctions, arrows for causality (X → Y), one word when one word enough.
	// Be cautious and ensure you've committed your memory files before using this mode - as we tell our kids: "You get what you get, and you don't throw a fit"
	CavemanUltra
)

func (m CavemanMode) String() string {
	switch m {
	case CavemanOff:
		return "off"
	case CavemanLite:
		return "lite"
	case CavemanFull:
		return "full"
	case CavemanUltra:
		return "ultra"
	default:
		return "unknown"
	}
}

// ParseCavemanMode parses a string into a CavemanMode. Unknown values return CavemanOff.
func ParseCavemanMode(s string) CavemanMode {
	switch s {
	case "off":
		return CavemanOff
	case "lite":
		return CavemanLite
	case "full":
		return CavemanFull
	case "ultra":
		return CavemanUltra
	default:
		return CavemanOff
	}
}

// Option is a functional option for configuring Compacter.
type Option func(*Compacter)

// WithLogger sets the logger for the Compacter.
func WithLogger(logger *slog.Logger) Option {
	return func(c *Compacter) {
		c.logger = logger
	}
}

// WithCavemanMode enables caveman mode, which Julius Brussee's prompt for reduced token size. Use at your own risk
// (make sure memory is committed beforehand).
//
// see: https://juliusbrussee.github.io/caveman/
func WithCavemanMode(mode CavemanMode) Option {
	return func(c *Compacter) {
		if mode < CavemanOff || mode > CavemanUltra {
			mode = CavemanOff
		}
		c.cavemanMode = mode
	}
}
