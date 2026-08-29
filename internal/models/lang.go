package models

import (
	"context"
	"strconv"
	"strings"
)

// Lang is a two-letter language code of the dictionaries' translations.
type Lang string

const (
	LangRU Lang = "ru"
	LangEN Lang = "en"

	// DefaultLang is used when the client does not ask for a supported
	// language; it is also the fallback when a translation is missing.
	DefaultLang = LangRU
)

// SupportedLangs lists the languages the dictionaries are translated to.
var SupportedLangs = []Lang{LangRU, LangEN}

// IsSupported reports whether translations exist for the language.
func (l Lang) IsSupported() bool {
	for _, s := range SupportedLangs {
		if s == l {
			return true
		}
	}
	return false
}

// ParseAcceptLanguage picks the best supported language from an
// Accept-Language header (RFC 9110 §12.5.4): tags are ordered by their q
// value (ties keep the header order), region subtags are ignored
// ("en-US" → "en") and unsupported tags are skipped. An empty or
// unparseable header yields DefaultLang.
func ParseAcceptLanguage(header string) Lang {
	type candidate struct {
		lang Lang
		q    float64
		pos  int
	}
	var best *candidate

	for pos, part := range strings.Split(header, ",") {
		fields := strings.Split(strings.TrimSpace(part), ";")
		tag := strings.ToLower(strings.TrimSpace(fields[0]))
		if tag == "" {
			continue
		}
		if i := strings.IndexAny(tag, "-_"); i > 0 {
			tag = tag[:i]
		}
		lang := Lang(tag)
		if !lang.IsSupported() {
			continue
		}

		q := 1.0
		for _, p := range fields[1:] {
			p = strings.TrimSpace(p)
			if v, ok := strings.CutPrefix(p, "q="); ok {
				if f, err := strconv.ParseFloat(v, 64); err == nil {
					q = f
				}
			}
		}
		if q <= 0 {
			continue
		}
		if best == nil || q > best.q {
			best = &candidate{lang: lang, q: q, pos: pos}
		}
	}

	if best == nil {
		return DefaultLang
	}
	return best.lang
}

type langKey struct{}

// ContextWithLang records the language the client asked for.
func ContextWithLang(ctx context.Context, lang Lang) context.Context {
	return context.WithValue(ctx, langKey{}, lang)
}

// LangFromContext returns the requested language or DefaultLang.
func LangFromContext(ctx context.Context) Lang {
	if lang, ok := ctx.Value(langKey{}).(Lang); ok && lang.IsSupported() {
		return lang
	}
	return DefaultLang
}
