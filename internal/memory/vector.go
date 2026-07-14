package memory

import (
	"encoding/binary"
	"math"
)

// packFloat32 encodes a float32 slice as little-endian bytes.
func packFloat32(v []float32) []byte {
	if len(v) == 0 {
		return nil
	}
	b := make([]byte, 4*len(v))
	for i, f := range v {
		binary.LittleEndian.PutUint32(b[i*4:], math.Float32bits(f))
	}
	return b
}

// unpackFloat32 decodes little-endian float32 bytes.
func unpackFloat32(b []byte) []float32 {
	if len(b) < 4 || len(b)%4 != 0 {
		return nil
	}
	n := len(b) / 4
	v := make([]float32, n)
	for i := 0; i < n; i++ {
		v[i] = math.Float32frombits(binary.LittleEndian.Uint32(b[i*4:]))
	}
	return v
}

// cosineSim returns cosine similarity in [-1, 1]. Zero vectors → 0.
func cosineSim(a, b []float32) float64 {
	if len(a) == 0 || len(a) != len(b) {
		return 0
	}
	var dot, na, nb float64
	for i := range a {
		fa := float64(a[i])
		fb := float64(b[i])
		dot += fa * fb
		na += fa * fa
		nb += fb * fb
	}
	if na == 0 || nb == 0 {
		return 0
	}
	return dot / (math.Sqrt(na) * math.Sqrt(nb))
}

// snippetFromText returns a short preview of text.
func snippetFromText(s string, max int) string {
	s = collapseWS(s)
	if max <= 0 {
		max = 160
	}
	if len(s) <= max {
		return s
	}
	return s[:max-1] + "…"
}

func collapseWS(s string) string {
	var b []byte
	space := false
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c == ' ' || c == '\n' || c == '\t' || c == '\r' {
			if !space && len(b) > 0 {
				b = append(b, ' ')
				space = true
			}
			continue
		}
		space = false
		b = append(b, c)
	}
	return string(b)
}
